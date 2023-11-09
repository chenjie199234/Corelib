package redis

import (
	"context"
	"errors"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"

	gredis "github.com/redis/go-redis/v9"
)

//group(redis sorted set): task1(priority1) task2(priority2) task3(priority3)... // if priority <0 this task's is interrupted,can't sub,but can pub
//{group}_task1(redis string):1		//this is the finishing pub status for task1,can't pub
//{group}_task1_channel1(redis list):[data1,data2,data3...]
//{group}_task1_channel2(redis list):[data1,data2,data3...]
//...
//{group}_task2(redis string):1		//this is the finishing pub status for task2
//{group}_task2_channel1(redis list):[data1,data2,data3...]
//{group}_task2_channel2(redis list):[data1,data2,data3...]
//...
//{group}_task3(redis string):1		//this is the finishing pub status for task3
//{group}_task3_channel1(redis list):[data1,data2,data3...]
//{group}_task3_channel2(redis list):[data1,data2,data3...]

// tasks in one same group will compete by priority
// tasks in different groups have no competition
// one specific group will only work on one redis node when in cluster mode
// different groups may work on different redis node when in cluster mode(depend on the group name)

var pubPMQ *gredis.Script
var finishPMQ *gredis.Script

func init() {
	pubPMQ = gredis.NewScript(`local task=string.sub(KEYS[2],#KEYS[1]+4)
if(not redis.call("ZSCORE",KEYS[1],task))
then
	return -2
end
if(redis.call("EXISTS",KEYS[2])==1)
then
	return -1
end
redis.call("RPUSH",KEYS[3],unpack(ARGV))
return 0`)

	finishPMQ = gredis.NewScript(`if(not redis.call("ZSCORE",KEYS[1],ARGV[1]))
then
	redis.call("DEL",KEYS[2])
	for i=3,#KEYS,1 do
		redis.call("DEL",KEYS[i])
	end
	return 1
end
for i=3,#KEYS,1 do
	if(redis.call("LLEN",KEYS[i])>0)
	then
		redis.call("SET",KEYS[2],1)
		return 0
	end
end
redis.call("ZREM",KEYS[1],ARGV[1])
redis.call("DEL",KEYS[2])
return 1`)
}

var ErrPriorityMQGroupMissing = errors.New("priority mq group missing")
var ErrPriorityMQTaskMissing = errors.New("priority mq task missing")
var ErrPriorityMQChannelMissing = errors.New("priority mq channel missing")
var ErrPriorityMQTaskFinishing = errors.New("priority mq task is finishing")
var ErrPriorityMQTaskFinished = errors.New("priority mq task finished")

// priority - the bigger the number is ranked previous
// if priority <0,means this task is interrupted,can't sub,but can pub
func (c *Client) PriorityMQSetTask(ctx context.Context, group, task string, priority int64) error {
	if group == "" {
		return ErrPriorityMQGroupMissing
	}
	if task == "" {
		return ErrPriorityMQTaskMissing
	}
	_, e := c.ZAdd(ctx, group, gredis.Z{Score: float64(priority), Member: task}).Result()
	return e
}

// return key - task,value - priority
func (c *Client) PriorityMQGetCurTasks(ctx context.Context, group string) (map[string]int64, error) {
	if group == "" {
		return nil, ErrPriorityMQGroupMissing
	}
	r, e := c.ZRangeWithScores(ctx, group, 0, -1).Result()
	if e != nil {
		return nil, e
	}
	result := make(map[string]int64, len(r))
	for _, v := range r {
		result[v.Member.(string)] = int64(v.Score)
	}
	return result, nil
}

// this function should be call by the puber in a loop,until this function return 1
// return 1 means task finished
// return 0 means task is finishing,(channel still has data)
func (c *Client) PriorityMQFinishTaskPub(ctx context.Context, group, task string, usedchannels ...string) (int, error) {
	if group == "" {
		return 0, ErrPriorityMQGroupMissing
	}
	if task == "" {
		return 0, ErrPriorityMQTaskMissing
	}
	if len(usedchannels) == 0 {
		return 0, ErrPriorityMQChannelMissing
	}
	keys := make([]string, 0, len(usedchannels)+2)
	taskkey := "{" + group + "}_" + task
	keys = append(keys, group, taskkey)
	for _, channel := range usedchannels {
		keys = append(keys, "{"+group+"}_"+task+"_"+channel)
	}
	return finishPMQ.Run(ctx, c, keys, task).Int()
}

func (c *Client) PriorityMQPub(ctx context.Context, group, task, channel string, datas ...interface{}) error {
	if group == "" {
		return ErrPriorityMQGroupMissing
	}
	if task == "" {
		return ErrPriorityMQTaskMissing
	}
	if channel == "" {
		return ErrPriorityMQChannelMissing
	}
	if len(datas) == 0 {
		return nil
	}
	taskkey := "{" + group + "}_" + task
	channelkey := "{" + group + "}_" + task + "_" + channel
	r, e := pubPMQ.Run(ctx, c, []string{group, taskkey, channelkey}, datas...).Int()
	if e != nil {
		return e
	}
	if r == -1 {
		return ErrPriorityMQTaskFinishing
	}
	if r == -2 {
		return ErrPriorityMQTaskFinished
	}
	return nil
}

func (c *Client) PriorityMQSub(group, channel string, subhandler func(task string, data []byte)) (stop func(), e error) {
	if group == "" {
		return nil, ErrPriorityMQGroupMissing
	}
	if channel == "" {
		return nil, ErrPriorityMQChannelMissing
	}
	ctx, cancel := context.WithCancel(context.Background())
	wait := make(chan *struct{}, 1)
	stop = func() {
		cancel()
		<-wait
	}
	go func() {
		c.priorityMQSubHandle(ctx, group, channel, subhandler)
		wait <- nil
	}()
	return
}
func (c *Client) priorityMQSubHandle(ctx context.Context, group, channel string, handle func(string, []byte)) {
	var tasks []string
	var result []string
	var e error
	for {
		if e != nil && ctx.Err() == nil {
			time.Sleep(time.Millisecond * 10)
		}
		if ctx.Err() != nil {
			//stopped
			return
		}
		tasks, e = c.ZRevRangeByScore(ctx, group, &gredis.ZRangeBy{Min: "0", Max: "+inf"}).Result()
		if e != nil {
			log.Error(ctx, "[redis.priorityMQSubHandle] get tasks failed", log.String("group", group), log.CError(e))
			continue
		}
		if len(tasks) == 0 && ctx.Err() == nil {
			time.Sleep(time.Second)
			continue
		}
		if ctx.Err() != nil {
			//stopped
			return
		}
		keys := make([]string, 0, len(tasks))
		for _, task := range tasks {
			keys = append(keys, "{"+group+"}_"+task+"_"+channel)
		}
		if result, e = c.BLPop(ctx, time.Second, keys...).Result(); e == nil {
			handle(result[0][len(group)+3:len(result[0])-len(channel)-1], common.STB(result[1]))
		} else if ee, ok := e.(interface{ Timeout() bool }); (!ok || !ee.Timeout()) && e != gredis.Nil {
			log.Error(ctx, "[redis.priorityMQSubHandle] sub tasks failed", log.String("group", group), log.String("channel", channel), log.CError(e))
		} else {
			e = nil
		}
	}
}
