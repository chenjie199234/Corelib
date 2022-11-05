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

const secondmax = `local time=redis.call("TIME")
for i=1,#KEYS,1 do
	while(true)
	do
		local first=redis.call("LINDEX",KEYS[i],0)
		if(first==false or tonumber(first)>time[1]*1000000+time[2])
		then
			break
		end
		redis.call("LPOP",KEYS[i])
	end
	local num=tonumber(redis.call("LLEN",KEYS[i]))
	local max=tonumber(ARGV[i])
	if(num>=max) then
		return 0
	end
end
for i=1,#KEYS,1 do
	redis.call("EXPIRE",KEYS[i],100)
	redis.call("RPUSH",KEYS[i],(time[1]+1)*1000000+time[2])
	redis.call("EXPIRE",KEYS[i],100)
end
return 1`

var hsecondmax = ""

// rates key: source name,value: max rate per second
// if rates have multi key and value pairs means all source name should pass it's rate check at the same time
// return true-all pass,false-at least one busy
func (p *Pool) RateLimitSecondMax(ctx context.Context, rates map[string]uint64) (bool, error) {
	args := make([]interface{}, len(rates)*2+2)
	args[0] = hsecondmax
	args[1] = len(rates)
	index := 2
	for k, v := range rates {
		if v == 0 {
			return false, nil
		}
		args[index] = k
		args[index+len(rates)] = v
		index++
	}
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVALSHA", args...))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		args[0] = secondmax
		r, e = redis.Int(c.(redis.ConnWithContext).DoContext(ctx, "EVAL", args...))
	}
	if e != nil {
		return false, e
	}
	return r == 1, nil
}
