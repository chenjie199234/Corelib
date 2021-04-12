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

type ListMQ struct {
	p    *Pool
	name string
	num  uint64
}

func init() {
	expiresha1 := sha1.Sum(common.Str2byte(expire))
	hexpire = hex.EncodeToString(expiresha1[:])

	pubsha1 := sha1.Sum(common.Str2byte(pub))
	hpub = hex.EncodeToString(pubsha1[:])
}

//listnum decide there are how many lists will be used in redis for this mq
//this is useful to balance all redis nodes' request
//every special list will have a name like mqname_[0,listnum)
//default is 10
func (p *Pool) NewListMQ(mqname string, listnum uint64) (*ListMQ, error) {
	if p.p == nil {
		return nil, errors.New("redis client not inited")
	}
	if mqname == "" {
		return nil, errors.New("mqname is empty")
	}
	if listnum == 0 {
		listnum = 10
	}
	return &ListMQ{p: p, name: mqname, num: listnum}, nil
}

const expire = `redis.call("SETEX","{"..KEYS[1].."}_exist",11,1)
redis.call("EXPIRE",KEYS[1],11)`

var hexpire = ""

func (l *ListMQ) Sub(recvbufnum uint64, stop chan struct{}) <-chan []byte {
	if recvbufnum < 1024 {
		recvbufnum = 1024
	}
	recv := make(chan []byte, recvbufnum)
	for i := uint64(0); i < l.num; i++ {
		index := i
		go func() {
			listname := l.name + "_" + strconv.FormatUint(index, 10)
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
					conn, e = l.p.GetContext(ctx)
					if e != nil {
						log.Error("[Redis.ListMQ.Sub.init] index:", index, "connect to redis error:", e)
						conn = nil
						cancel()
						continue
					}
					if _, e = conn.DoContext(ctx, "EVALSHA", hexpire, 1, listname); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
						_, e = conn.DoContext(ctx, "EVAL", expire, 1, listname)
					}
					if e != nil {
						log.Error("[Redis.ListMQ.Sub.init] index:", index, "exec error:", e)
						conn.Close()
						conn = nil
					}
					cancel()
				}
				for {
					//sub
					var data [][]byte
					if data, e = redis.ByteSlices(conn.c.(redis.ConnWithTimeout).DoWithTimeout(0, "BLPOP", listname, 0)); e != nil {
						log.Error("[Redis.ListMQ.Sub.sub] index:", index, "exec error:", e)
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
			listname := l.name + "_" + strconv.FormatUint(index, 10)
			tker := time.NewTicker(time.Second * 5)
			for {
				select {
				case <-tker.C:
					ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
					conn, e := l.p.GetContext(ctx)
					if e != nil {
						log.Error("[Redis.ListMQ.Sub.update] index:", index, "connect to redis error:", e)
						cancel()
						continue
					}
					if _, e = conn.DoContext(ctx, "EVALSHA", hexpire, 1, listname); e != nil && strings.Contains(e.Error(), "NOSCRIPT") {
						_, e = conn.DoContext(ctx, "EVAL", expire, 1, listname)
					}
					if e != nil {
						log.Error("[Redis.ListMQ.Sub.update] index:", index, "exec error:", e)
					}
					conn.Close()
					cancel()
				case <-stop:
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					if conn, e := l.p.GetContext(ctx); e != nil {
						log.Error("[Redis.ListMQ.Sub.stop] index:", index, "connect to redis error:", e)
					} else if _, e = conn.DoContext(ctx, "DEL", "{"+listname+"}_exist"); e != nil && e != redis.ErrNil {
						log.Error("[Redis.ListMQ.Sub.stop] index:", index, "exec error:", e)
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
	return recv
}

const pub = `if(redis.call("EXISTS","{"..KEYS[1].."}_exist")~=0)
then
	redis.call("rpush",KEYS[1],ARGV[1])
	redis.call("EXPIRE",KEYS[1],11)
	return 1
end
return 0`

var hpub = ""

//key is used to decide which list will the value send to
func (l *ListMQ) Pub(ctx context.Context, mqname string, listnum uint64, key string, value []byte) error {
	c, e := l.p.GetContext(ctx)
	if e != nil {
		return e
	}
	listname := mqname + "_" + strconv.FormatUint(common.BkdrhashString(key, listnum), 10)
	var r int
	if r, e = redis.Int(c.DoContext(ctx, "EVALSHA", hpub, 1, listname, value)); e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(c.DoContext(ctx, "EVAL", pub, 1, listname, value))
	}
	if e == nil && r == 0 {
		e = redis.ErrNil
	}
	return e
}
