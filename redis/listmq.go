package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gomodule/redigo/redis"
)

func init() {
	expiresha1 := sha1.Sum(common.Str2byte(expirelistmq))
	hexpirelistmq = hex.EncodeToString(expiresha1[:])

	pubsha1 := sha1.Sum(common.Str2byte(publistmq))
	hpublistmq = hex.EncodeToString(pubsha1[:])
}

var ErrListMQMissingName = errors.New("list mq missing name")
var ErrListMQMissingGroup = errors.New("list mq missing group")

const expirelistmq = `redis.call("SETEX",KEYS[2],11,1)
redis.call("EXPIRE",KEYS[1],11)`

var hexpirelistmq = ""

//groupnum decide there are how many lists will be used in redis for this mq
//	every special list will have a name like mqname_[0,groupnum)
//	in cluster mode:this is useful to balance all redis nodes' request
//	in slave mode:set it to 1
//	pub's groupnum must same as the sub's groupnum
func (p *Pool) ListMQSub(mqname string, groupnum uint64, subhandler func([]byte)) (func(), error) {
	if mqname == "" {
		return nil, ErrListMQMissingName
	}
	if groupnum == 0 {
		return nil, ErrListMQMissingGroup
	}
	stop := make(chan *struct{})
	cancel := func() {
		close(stop)
	}
	for i := uint64(0); i < groupnum; i++ {
		index := i
		listname := mqname + "_" + strconv.FormatUint(index, 10)
		listnameexist := "{" + listname + "}_exist"
		go func() {
			var c redis.Conn
			var e error
			for {
				for c == nil {
					//init
					if e != nil {
						//reconnect
						time.Sleep(time.Millisecond)
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					c, e = p.p.GetContext(ctx)
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.init] index:", index, "connect to redis error:", e)
						c = nil
						cancel()
						continue
					}
					cctx := c.(redis.ConnWithContext)
					if _, e = cctx.DoContext(ctx, "EVALSHA", hexpirelistmq, 2, listname, listnameexist); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
						_, e = cctx.DoContext(ctx, "EVAL", expirelistmq, 2, listname, listnameexist)
					}
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.init] index:", index, "exec error:", e)
						c.Close()
						c = nil
					}
					cancel()
				}
				cc := c.(redis.ConnWithTimeout)
				for {
					//sub
					var data [][]byte
					if data, e = redis.ByteSlices(cc.DoWithTimeout(0, "BLPOP", listname, 0)); e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.sub] index:", index, "exec error:", e)
						c.Close()
						c = nil
						break
					}
					if subhandler != nil {
						subhandler(data[1])
					}
				}
			}
		}()
		go func() {
			//update
			tker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-tker.C:
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
					c, e := p.p.GetContext(ctx)
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.update] index:", index, "connect to redis error:", e)
						cancel()
						continue
					}
					cctx := c.(redis.ConnWithContext)
					if _, e = cctx.DoContext(ctx, "EVALSHA", hexpirelistmq, 2, listname, listnameexist); e != nil && strings.Contains(e.Error(), "NOSCRIPT") {
						_, e = cctx.DoContext(ctx, "EVAL", expirelistmq, 2, listname, listnameexist)
					}
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.update] index:", index, "exec error:", e)
					}
					c.Close()
					cancel()
				case <-stop:
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					if c, e := p.p.GetContext(ctx); e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.stop] index:", index, "connect to redis error:", e)
						time.Sleep(5 * time.Millisecond)
					} else if _, e = c.(redis.ConnWithContext).DoContext(ctx, "DEL", listnameexist); e != nil && e != redis.ErrNil {
						log.Error(nil, "[Redis.ListMQ.Sub.stop] index:", index, "exec error:", e)
						c.Close()
						time.Sleep(5 * time.Millisecond)
					} else {
						c.Close()
						cancel()
						return
					}
					cancel()
				}
			}
		}()
	}
	return cancel, nil
}

const publistmq = `if(redis.call("EXISTS",KEYS[2])~=0)
then
	redis.call("EXPIRE",KEYS[1],11)
	for i=1,#ARGV,1 do
		redis.call("rpush",KEYS[1],ARGV[i])
	end
	return 1
end
return 0`

var hpublistmq = ""

//groupnum decide there are how many lists will be used in redis for this mq
//	every special list will have a name like mqname_[0,groupnum)
//	in cluster mode:this is useful to balance all redis nodes' request
//	in slave mode:set it to 1
//	pub's groupnum must same as the sub's groupnum
//key is used to decide which list will the value send to
func (p *Pool) ListMQPub(ctx context.Context, mqname string, groupnum uint64, key string, values ...[]byte) error {
	if groupnum == 0 {
		return redis.ErrNil
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	listname := mqname + "_" + strconv.FormatUint(common.BkdrhashString(key, groupnum), 10)
	listnameexist := "{" + listname + "}_exist"
	args := make([]interface{}, 0, 4+len(values))
	args = append(args, hpublistmq)
	args = append(args, 2)
	args = append(args, listname)
	args = append(args, listnameexist)
	for _, value := range values {
		args = append(args, value)
	}
	var r int
	cctx := c.(redis.ConnWithContext)
	if r, e = redis.Int(cctx.DoContext(ctx, "EVALSHA", args...)); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = publistmq
		r, e = redis.Int(cctx.DoContext(ctx, "EVAL", args...))
	}
	if e == nil && r == 0 {
		e = redis.ErrNil
	}
	return e
}
