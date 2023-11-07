package redis

import (
	"context"
	"errors"
	"strconv"

	"github.com/chenjie199234/Corelib/util/common"

	gredis "github.com/redis/go-redis/v9"
)

var ErrVerifyCodeCheckTimesUsedup = errors.New("all check times used up")
var ErrVerifyCodeReceiverMissing = errors.New("receiver not exist")
var ErrVerifyCodeMissing = errors.New("verify code not exist")
var ErrVerifyCodeWrong = errors.New("verify code wrong")
var makevcode *gredis.Script
var checkvcode *gredis.Script

func init() {
	makevcode = gredis.NewScript(`local addReceiver=0
if(ARGV[2]~="")
then
	addReceiver=redis.call("HSETNX",KEYS[1],ARGV[2],1)
end
local data=redis.call("HMGET",KEYS[1],"_code","_check")
if(data[1] and data[2])
then
	if(tonumber(data[2])>=5)
	then
		return {data[1],tonumber(data[2]),addReceiver}
	end
	if(addReceiver==1)
	then
		redis.call("EXPIRE",KEYS[1],ARGV[3])
	end
	return {data[1],tonumber(data[2]),addReceiver}
end
redis.call("HMSET",KEYS[1],"_code",ARGV[1],"_check",0)
redis.call("EXPIRE",KEYS[1],ARGV[3])
return {ARGV[1],0,addReceiver}`)

	checkvcode = gredis.NewScript(`local data=redis.call("HMGET",KEYS[1],"_code","_check",ARGV[2])
if(not data[1] or not data[2])
then
	return nil
end
if(tonumber(data[2])>=5)
then
	return 5
end
data[2]=redis.call("HINCRBY",KEYS[1],"_check",1)
if(ARGV[2]~="" and not data[3])
then
	return -1
end
if(data[1]==ARGV[1])
then
	redis.call("DEL",KEYS[1])
	return 0
end
return tonumber(data[2])`)
}

// target + action make the unique key in redis and CheckVerifyCode(if check pass) or DelVerifyCode will delete this key
// have 5 times for check
// expire unit is second and it will only be used when new receiver added into this key to share the code
// in the key's life,different receiver add to this key will get the same code
func (c *Client) MakeVerifyCode(ctx context.Context, target, action, receiver string, expire uint) (code string, duplicatereceiver bool, e error) {
	key := "verify_code_{" + target + "}_" + action
	data, e := makevcode.Run(ctx, c, []string{key}, common.MakeRandCode(6), receiver, expire).Slice()
	if e != nil {
		return "", false, e
	}
	code = data[0].(string)
	duplicatereceiver = receiver != "" && data[2].(int64) == 0
	if data[1].(int64) == 5 {
		e = ErrVerifyCodeCheckTimesUsedup
	}
	return
}

// target + action make the unique key in redis and the key will be deleted if check pass
// if mustreceiver is not empty,the check can pass only if this receiver exists
// have 5 times for check
func (c *Client) CheckVerifyCode(ctx context.Context, target, action, code, mustreceiver string) error {
	key := "verify_code_{" + target + "}_" + action
	r, e := checkvcode.Run(ctx, c, []string{key}, code, mustreceiver).Int()
	if e != nil {
		if e == gredis.Nil {
			e = ErrVerifyCodeMissing
		}
		return e
	}
	if r == -1 {
		return ErrVerifyCodeReceiverMissing
	}
	if r == 5 {
		return ErrVerifyCodeCheckTimesUsedup
	}
	if r != 0 {
		return ErrVerifyCodeWrong
	}
	return nil
}

// return nil means has check times
// return ErrVerifyCodeMissing means verify code doesn't exist
// return ErrVerifyCodeCheckTimesUsedup means no check times any more
// return ErrVerifyCodeReceiverMissing means the must receiver doesn't exist
func (c *Client) HasCheckTimes(ctx context.Context, target, action, mustreceiver string) error {
	key := "verify_code_{" + target + "}_" + action
	rs, e := c.HMGet(ctx, key, "_check", mustreceiver).Result()
	if e != nil {
		return e
	}
	if rs[0] == nil {
		return ErrVerifyCodeMissing
	} else if r, e := strconv.Atoi(rs[0].(string)); e != nil {
		return e
	} else if r >= 5 {
		return ErrVerifyCodeCheckTimesUsedup
	}
	if mustreceiver != "" && rs[1] == nil {
		return ErrVerifyCodeReceiverMissing
	}
	return nil
}

func (c *Client) DelVerifyCode(ctx context.Context, target, action string) error {
	key := "verify_code_{" + target + "}_" + action
	_, e := c.Del(ctx, key).Result()
	return e
}
