package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gomodule/redigo/redis"
)

//all tasks in one group will compete by priority
//one specific group will only work on one redis node when in cluster mode
//tasks in different groups has no compete
//different groups may work on different redis node when in cluster mode(depend on the group name)

func init() {
	pubprioritymqsha1 := sha1.Sum(common.Str2byte(pubprioritymq))
	hpubprioritymq = hex.EncodeToString(pubprioritymqsha1[:])

	finishprioritymqsha1 := sha1.Sum(common.Str2byte(finishprioritymq))
	hfinishprioritymq = hex.EncodeToString(finishprioritymqsha1[:])
}

var ErrPriorityMQMissingGroup = errors.New("priority mq missing group")

// priority - the bigger the number is ranked previous
func (p *Pool) PriorityMQSetTask(ctx context.Context, group, taskname string, priority uint64) error {
	if group == "" {
		return ErrPriorityMQMissingGroup
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	_, e = c.(redis.ConnWithContext).DoContext(ctx, "ZADD", group, priority, taskname)
	return e
}

func (p *Pool) PriorityMQInterrupt(ctx context.Context, group, taskname string) error {
	if group == "" {
		return ErrPriorityMQMissingGroup
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	_, e = c.(redis.ConnWithContext).DoContext(ctx, "ZREM", group, taskname)
	return e
}

// return key - taskname,value - priority
func (p *Pool) PriorityMQGetCurTasks(ctx context.Context, group string) (map[string]uint64, error) {
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	return redis.Uint64Map(c.(redis.ConnWithContext).DoContext(ctx, "ZRANGE", group, 0, -1, "WITHSCORES"))
}

var finishprioritymq = `local exist=redis.call("ZSCORE",KEYS[1],ARGV[1])
if(exist==nil)
then
	redis.call("DEL",KEYS[2])
	for i=3,#KEYS,1 do
		redis.call("DEL",KEYS[i])
	end
	return 1
end
redis.call("SET",KEYS[2],1)
for i=3,#KEYS,1 do
	local len=redis.call("LLEN",KEYS[i])
	if(len>0)
	then
		return 0
	end
end
redis.call("ZREM",KEYS[1],ARGV[1])
redis.call("DEL",KEYS[2])
return 1`
var hfinishprioritymq = ""

// this function should be call by the puber
// return 1 means task finished
// return 0 means task still working
func (p *Pool) PriorityMQFinishTask(ctx context.Context, group, taskname string, topicnames ...string) (int, error) {
	if len(topicnames) == 0 {
		return 0, nil
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return 0, e
	}
	defer c.Close()
	args := make([]interface{}, 0, 2+2+len(topicnames))
	args = append(args, hfinishprioritymq, 2+len(topicnames))
	taskkey := "{" + group + "}_" + taskname
	args = append(args, group, taskkey)
	for _, topicname := range topicnames {
		topickey := "{" + group + "}_" + taskname + "_" + topicname
		args = append(args, topickey)
	}
	args = append(args, taskname)
	var r int
	if r, e = redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", args...)); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = finishprioritymq
		_, e = c.(redis.ConnWithContext).DoContext(ctx, "EVAL", args...)
	}
	return r, e
}

var pubprioritymq = `local exist=redis.call("EXISTS",KEYS[2])
if(exist==1)
then
	return -1
end
for i=2,#ARGV,1 do
	redis.call("RPUSH",KEYS[3],ARGV[i])
end
return 0`
var hpubprioritymq = ""

var ErrPriorityMQTaskFinished = errors.New("task finished")

func (p *Pool) PriorityMQPub(ctx context.Context, group, taskname, topicname string, datas ...[]byte) error {
	if len(datas) == 0 {
		return nil
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	taskkey := "{" + group + "}_" + taskname
	topickey := "{" + group + "}_" + taskname + "_" + topicname
	args := make([]interface{}, 0, 6+len(datas))
	args = append(args, hpubprioritymq, 3, group, taskkey, topickey, taskname)
	for _, data := range datas {
		args = append(args, data)
	}
	r, e := redis.Int64(c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", args...))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = pubprioritymq
		r, e = redis.Int64(c.(redis.ConnWithContext).DoContext(ctx, "EVAL", args...))
	}
	if e != nil {
		return e
	}
	if r == -1 {
		return ErrPriorityMQTaskFinished
	}
	return nil
}

var ErrPriorityMQMissingTopic = errors.New("priority mq missing topic")

func (p *Pool) PriorityMQSub(group, topicname string, subhandler func(taskname, data string)) (cancel func(), e error) {
	if group == "" {
		return nil, ErrPriorityMQMissingGroup
	}
	if topicname == "" {
		return nil, ErrPriorityMQMissingTopic
	}
	var c redis.Conn
	status := 0 //0-working,1-cancel
	finish := make(chan *struct{})
	cancel = func() {
		status = 1
		<-finish
	}
	go func() {
		defer close(finish)
		var e error
		for {
			if e != nil {
				if status == 1 {
					break
				}
				//reconnect
				time.Sleep(time.Millisecond * 10)
			}
			if status == 1 {
				break
			}
			c, e = p.p.GetContext(context.Background())
			if e != nil {
				log.Error(nil, "[redis.PriorityMQ.sub] get connection error:", e)
				continue
			}
			for {
				if status == 1 {
					break
				}
				var datas []string
				datas, e = redis.Strings(c.(redis.ConnWithTimeout).DoWithTimeout(0, "ZREVRANGE", group, 0, -1))
				if e != nil {
					log.Error(nil, "[redis.PriorityMQ.sub] get tasks error:", e)
					break
				}
				if len(datas) == 0 {
					//no task,loop
					time.Sleep(time.Second)
					continue
				}
				if status == 1 {
					break
				}
				args := make([]interface{}, 0, len(datas)+1)
				for _, taskname := range datas {
					args = append(args, "{"+group+"}_"+taskname+"_"+topicname)
				}
				args = append(args, 1)
				datas, e = redis.Strings(c.(redis.ConnWithTimeout).DoWithTimeout(0, "BLPOP", args...))
				if e != nil {
					if e == redis.ErrNil {
						//timeout
						continue
					}
					log.Error(nil, "[redis.PriorityMQ.sub] BLPOP error:", e)
					break
				}
				if subhandler != nil {
					subhandler(datas[0], datas[1])
				}
			}
			c.Close()
		}
	}()
	return
}
