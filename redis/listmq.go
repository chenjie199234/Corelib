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
	expiresha1 := sha1.Sum(common.Str2byte(expire))
	hexpire = hex.EncodeToString(expiresha1[:])

	pubsha1 := sha1.Sum(common.Str2byte(pub))
	hpub = hex.EncodeToString(pubsha1[:])
}

const expire = `redis.call("SETEX",KEYS[2],11,1)
redis.call("EXPIRE",KEYS[1],11)`

var hexpire = ""

//num decide there are how many lists will be used in redis for this mq
//this is useful to balance all redis nodes' request
//every special list will have a name like name_[0,num)
//default is 10
func (p *Pool) ListMQSub(name string, num uint64, recvbufnum uint64, stop chan struct{}) (<-chan []byte, error) {
	if name == "" {
		return nil, errors.New("mq name is empty")
	}
	if num == 0 {
		num = 10
	}
	if recvbufnum < 1024 {
		recvbufnum = 1024
	}
	recv := make(chan []byte, recvbufnum)
	for i := uint64(0); i < num; i++ {
		index := i
		listname := name + "_" + strconv.FormatUint(index, 10)
		listnameexist := "{" + listname + "}_exist"
		go func() {
			var conn *Conn
			var e error
			for {
				for conn == nil {
					//init
					if e != nil {
						//reconnect
						time.Sleep(time.Millisecond)
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					conn, e = p.GetContext(ctx)
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.init] index:", index, "connect to redis error:", e)
						conn = nil
						cancel()
						continue
					}
					if _, e = conn.DoContext(ctx, "EVALSHA", hexpire, 2, listname, listnameexist); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
						_, e = conn.DoContext(ctx, "EVAL", expire, 2, listname, listnameexist)
					}
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.init] index:", index, "exec error:", e)
						conn.Close()
						conn = nil
					}
					cancel()
				}
				for {
					//sub
					var data [][]byte
					if data, e = redis.ByteSlices(conn.c.(redis.ConnWithTimeout).DoWithTimeout(0, "BLPOP", listname, 0)); e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.sub] index:", index, "exec error:", e)
						conn.Close()
						conn = nil
						break
					}
					recv <- data[1]
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
					conn, e := p.GetContext(ctx)
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.update] index:", index, "connect to redis error:", e)
						cancel()
						continue
					}
					if _, e = conn.DoContext(ctx, "EVALSHA", hexpire, 2, listname, listnameexist); e != nil && strings.Contains(e.Error(), "NOSCRIPT") {
						_, e = conn.DoContext(ctx, "EVAL", expire, 2, listname, listnameexist)
					}
					if e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.update] index:", index, "exec error:", e)
					}
					conn.Close()
					cancel()
				case <-stop:
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					if conn, e := p.GetContext(ctx); e != nil {
						log.Error(nil, "[Redis.ListMQ.Sub.stop] index:", index, "connect to redis error:", e)
					} else if _, e = conn.DoContext(ctx, "DEL", listnameexist); e != nil && e != redis.ErrNil {
						log.Error(nil, "[Redis.ListMQ.Sub.stop] index:", index, "exec error:", e)
						conn.Close()
						time.Sleep(5 * time.Millisecond)
					} else {
						conn.Close()
						cancel()
						return
					}
					cancel()
				}
			}
		}()
	}
	return recv, nil
}

const pub = `if(redis.call("EXISTS",KEYS[2])~=0)
then
	redis.call("EXPIRE",KEYS[1],11)
	for i=1,#ARGV,1 do
		redis.call("rpush",KEYS[1],ARGV[i])
	end
	return 1
end
return 0`

var hpub = ""

//key is used to decide which list will the value send to
func (p *Pool) ListMQPub(ctx context.Context, name string, num uint64, key string, values ...[]byte) error {
	if num == 0 {
		num = 10
	}
	c, e := p.GetContext(ctx)
	if e != nil {
		return e
	}
	listname := name + "_" + strconv.FormatUint(common.BkdrhashString(key, num), 10)
	listnameexist := "{" + listname + "}_exist"
	args := make([]interface{}, 0, 4+len(values))
	args = append(args, hpub)
	args = append(args, 2)
	args = append(args, listname)
	args = append(args, listnameexist)
	for _, value := range values {
		args = append(args, value)
	}
	var r int
	if r, e = redis.Int(c.DoContext(ctx, "EVALSHA", args...)); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = pub
		r, e = redis.Int(c.DoContext(ctx, "EVAL", args...))
	}
	if e == nil && r == 0 {
		e = redis.ErrNil
	}
	return e
}
