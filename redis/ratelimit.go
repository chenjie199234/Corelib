package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

func init() {
	h := sha1.Sum([]byte(secondmax))
	hsecondmax = hex.EncodeToString(h[:])
}

const secondmax = `local first=redis.call("LINDEX",KEYS[1],0)
if(first=="null")
then
	redis.call("RPUSH",KEYS[1],ARGV[2]+1000000000)
	redis.call("EXPIRE",KEYS[1],1)
	return 1
end
if(tonumber(first)<=tonumber(ARGV[2]))
then
	redis.call("LPOP",KEYS[1])
end
if(tonumber(redis.call("LLEN",KEYS[1]))<tonumber(ARGV[1]))
then
	redis.call("RPUSH",KEYS[1],ARGV[2]+1000000000)
	redis.call("EXPIRE",KEYS[1],1)
	return 1
end
return 0`

var hsecondmax = ""

//return true-pass,false-busy
func (p *Pool) RateLimitSecondMax(ctx context.Context, key string, max uint64) (bool, error) {
	if max == 0 {
		return false, nil
	}
	c, e := p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	now := time.Now().UnixNano()
	r, e := redis.Int(c.DoContext(ctx, "EVALSHA", hsecondmax, key, max, now))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(c.DoContext(ctx, "EVAL", secondmax, key, max, now))
	}
	if e != nil {
		return false, e
	}
	return r == 1, nil
}
