package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gomodule/redigo/redis"
)

var ErrTemporaryMQMissingName = errors.New("temporary mq missing name")
var ErrTemporaryMQMissingGroup = errors.New("temporary mq missing group")

func init() {
	pubsha1 := sha1.Sum(common.Str2byte(pubTemporaryMQ))
	hpubTemporaryMQ = hex.EncodeToString(pubsha1[:])

	expiresha1 := sha1.Sum(common.Str2byte(expireTemporaryMQ))
	hexpireTemporaryMQ = hex.EncodeToString(expiresha1[:])
}

const expireTemporaryMQ = `redis.call("SETEX",KEYS[2],11,1)
redis.call("EXPIRE",KEYS[1],11)`

var hexpireTemporaryMQ = ""

//in redis cluster mode,group is used to shard data into different redis node
//in redis slave master mode,group is better to be 1
//sub and pub's group should be same
//cancel will stop the sub immediately,the mq's empty or not will not effect the cancel
// 	so there maybe data left in the mq,and it will be expired within 16s
func (p *Pool) TemporaryMQSub(name string, group uint64, subhandler func([]byte)) (cancel func(), e error) {
	if name == "" {
		return nil, ErrTemporaryMQMissingName
	}
	if group == 0 {
		return nil, ErrTemporaryMQMissingGroup
	}
	update := func(ctx context.Context) error {
		var err error
		wg := sync.WaitGroup{}
		for i := uint64(0); i < group; i++ {
			wg.Add(1)
			go func(index uint64) {
				defer wg.Done()
				listname := name + "_" + strconv.FormatUint(index, 10)
				listexist := "{" + listname + "}_exist"
				c, e := p.p.GetContext(context.Background())
				if e != nil {
					err = e
					log.Error(nil, "[redis.ListMQ.update] index:", index, "get connection error:", e)
					return
				}
				defer c.Close()
				if _, e = c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", hexpireTemporaryMQ, 2, listname, listexist); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
					_, e = c.(redis.ConnWithContext).DoContext(ctx, "EVAL", expireTemporaryMQ, 2, listname, listexist)
				}
				if e != nil {
					err = e
					log.Error(nil, "[redis.ListMQ.update] index:", index, "error:", e)
				}
			}(i)
			if i%20 == 19 {
				wg.Wait()
			}
		}
		wg.Wait()
		return err
	}
	if e := update(context.Background()); e != nil {
		return nil, e
	}
	wg := &sync.WaitGroup{}
	status := 0 //0-working,1-cancel
	tker := time.NewTicker(time.Second * 5)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer tker.Stop()
		for {
			<-tker.C
			if status != 0 {
				break
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
			if e := update(ctx); e != nil {
				tker.Reset(time.Millisecond * 500)
			} else {
				tker.Reset(time.Second * 5)
			}
			cancel()
			if len(tker.C) > 0 {
				<-tker.C
			}
			cancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		c, e := p.p.GetContext(ctx)
		if e != nil {
			log.Error(nil, "[redis.ListMQ.stop] get connection error:", e)
			return
		}
		defer c.Close()
		if _, e = c.(redis.ConnWithContext).DoContext(ctx, "DEL", name); e != nil {
			log.Error(nil, "[redis.ListMQ.stop] error:", e)
		}
		return
	}()
	lker := &sync.Mutex{}
	conns := make(map[redis.Conn]*struct{}, group)
	cancel = func() {
		status = 1
		tker.Reset(1)
		lker.Lock()
		for conn := range conns {
			conn.Close()
		}
		lker.Unlock()
		wg.Wait()
	}
	for i := uint64(0); i < group; i++ {
		wg.Add(1)
		go func(index uint64) {
			defer wg.Done()
			listname := name + "_" + strconv.FormatUint(index, 10)
			var c redis.Conn
			var e error
			for {
				if e != nil {
					if status != 0 {
						break
					}
					//reconnect
					time.Sleep(time.Millisecond * 10)
				}
				if status != 0 {
					break
				}
				c, e = redis.DialURL(p.c.URL, redis.DialConnectTimeout(p.c.ConnTimeout), redis.DialReadTimeout(p.c.IOTimeout), redis.DialWriteTimeout(p.c.IOTimeout))
				if e != nil {
					log.Error(nil, "[redis.ListMQ.sub] get connection error:", e)
					continue
				}
				lker.Lock()
				if status != 0 {
					c.Close()
					lker.Unlock()
					break
				}
				conns[c] = nil
				lker.Unlock()
				var data [][]byte
				for {
					data, e = redis.ByteSlices(c.(redis.ConnWithTimeout).DoWithTimeout(0, "BLPOP", listname, 0))
					if e != nil {
						if ee := errors.Unwrap(e); ee != nil && ee == net.ErrClosed && status == 1 {
							break
						}
						log.Error(nil, "[redis.ListMQ.sub] index:", index, "exec error:", e)
						break
					}
					if subhandler != nil {
						subhandler(data[1])
					}
				}
				lker.Lock()
				delete(conns, c)
				c.Close()
				lker.Unlock()
			}
		}(i)
	}
	return
}

const pubTemporaryMQ = `if(redis.call("EXISTS",KEYS[2])==0)
then
	return
end
redis.call("EXPIRE",KEYS[1],16)
for i=1,#ARGV,1 do
	redis.call("rpush",KEYS[1],ARGV[i])
end
redis.call("EXPIRE",KEYS[1],16)
return #ARGV`

var hpubTemporaryMQ = ""

//in redis cluster mode,group is used to shard data into different redis node
//in redis slave master mode,group is better to be 1
//sub and pub's group should be same
func (p *Pool) TemporaryMQPub(ctx context.Context, name string, group uint64, key string, value ...[]byte) error {
	if len(value) == 0 {
		return nil
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	listname := name + "_" + strconv.FormatUint(common.BkdrhashString(key, group), 10)
	listexist := "{" + listname + "}_exist"
	args := make([]interface{}, 0, 4+len(value))
	args = append(args, hpubTemporaryMQ, 2, listname, listexist)
	for _, v := range value {
		args = append(args, v)
	}
	if _, e = c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", args...); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = pubTemporaryMQ
		_, e = redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVAL", args...))
	}
	return e
}
