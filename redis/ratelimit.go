package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var rate *redis.Script

func init() {
	rate = redis.NewScript(`local time=redis.call("TIME")
for i=1,#KEYS,1 do
	while(true)
	do
		local first=redis.call("LINDEX",KEYS[i],0)
		if(not first or tonumber(first)>time[1]*1000000+time[2])
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
	local period=tonumber(ARGV[i+#KEYS])
	redis.call("RPUSH",KEYS[i],(time[1]+period)*1000000+time[2])
	redis.call("EXPIRE",KEYS[i],period)
end
return 1`)
}

// return true-all pass,false-at least one busy
// rates: key:source name,value(max rate per period):first element:max rate,second element:period(uint second)
// If rates have multi key and value pairs,means all source name should pass it's rate check at the same time
// Warning!In redis cluster mode,multi key should route to the same redis node!Please set the source name carefully
func (c *Client) RateLimit(ctx context.Context, rates map[string][2]uint64) (bool, error) {
	keys := make([]string, len(rates))
	values := make([]interface{}, len(rates)*2)
	i := 0
	for k, v := range rates {
		keys[i] = k
		values[i] = v[0]
		values[i+len(rates)] = v[1]
		i++
	}
	r, e := rate.Run(ctx, c, keys, values...).Int()
	if e != nil {
		return false, e
	}
	return r == 1, nil
}
