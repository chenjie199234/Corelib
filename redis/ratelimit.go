package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/gomodule/redigo/redis"
)

func init() {
	h := sha1.Sum([]byte(secondmax))
	hsecondmax = hex.EncodeToString(h[:])
}

const secondmax = `local first=redis.call("LINDEX",KEYS[1],0)
if(first==nil or first==false)
then
	local time=redis.call("TIME")
	redis.call("EXPIRE",KEYS[1],1)
	redis.call("RPUSH",KEYS[1],(time[1]+1)*1000000+time[2])
	redis.call("EXPIRE",KEYS[1],1)
	return 1
end
local time=redis.call("TIME")
if(tonumber(first)<=time[1]*1000000+time[2])
then
	redis.call("LPOP",KEYS[1])
end
if(tonumber(redis.call("LLEN",KEYS[1]))<tonumber(ARGV[1]))
then
	redis.call("EXPIRE",KEYS[1],1)
	redis.call("RPUSH",KEYS[1],(time[1]+1)*1000000+time[2])
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
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", hsecondmax, 1, key, max))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVAL", secondmax, 1, key, max))
	}
	if e != nil {
		return false, e
	}
	return r == 1, nil
}
