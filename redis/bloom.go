package redis

import (
	"context"
	"strconv"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/egroup"

	gredis "github.com/redis/go-redis/v9"
)

// {bloomname_1}_bkdr: redis bitset
// {bloomname_1}_djb: redis bitset
// {bloomname_1}_fnv: redis bitset
// {bloomname_1}_dev: redis bitset
// {bloomname_1}_rs: redis bitset
// {bloomname_1}_sdbm: redis bitset
// {bloomname_1}_exist: redis string
// ...
// {bloomname_n}_bkdr: redis bitset
// {bloomname_n}_djb: redis bitset
// {bloomname_n}_fnv: redis bitset
// {bloomname_n}_dev: redis bitset
// {bloomname_n}_rs: redis bitset
// {bloomname_n}_sdbm: redis bitset
// {bloomname_n}_exist: redis string

var initBloom *gredis.Script
var setBloom *gredis.Script
var checkBloom *gredis.Script

func init() {
	initBloom = gredis.NewScript(`if(redis.call("EXISTS",KEYS[7])==0)
then
	redis.call("SETBIT",KEYS[1],ARGV[1],1)
	redis.call("SETBIT",KEYS[2],ARGV[1],1)
	redis.call("SETBIT",KEYS[3],ARGV[1],1)
	redis.call("SETBIT",KEYS[4],ARGV[1],1)
	redis.call("SETBIT",KEYS[5],ARGV[1],1)
	redis.call("SETBIT",KEYS[6],ARGV[1],1)
	redis.call("SET",KEYS[7],1)
	local ex=tonumber(ARGV[2])
	if(ex>0)
	then
		redis.call("EXPIRE",KEYS[1],ex)
		redis.call("EXPIRE",KEYS[2],ex)
		redis.call("EXPIRE",KEYS[3],ex)
		redis.call("EXPIRE",KEYS[4],ex)
		redis.call("EXPIRE",KEYS[5],ex)
		redis.call("EXPIRE",KEYS[6],ex)
		redis.call("EXPIRE",KEYS[7],ex)
	end
	return 1
end
return 0`)

	setBloom = gredis.NewScript(`if(redis.call("EXISTS",KEYS[7])==0)
then
	return nil
end
local r1=redis.call("SETBIT",KEYS[1],ARGV[1],1)
local r2=redis.call("SETBIT",KEYS[2],ARGV[2],1)
local r3=redis.call("SETBIT",KEYS[3],ARGV[3],1)
local r4=redis.call("SETBIT",KEYS[4],ARGV[4],1)
local r5=redis.call("SETBIT",KEYS[5],ARGV[5],1)
local r6=redis.call("SETBIT",KEYS[6],ARGV[6],1)
if(redis.call("EXISTS",KEYS[7])==0)
then
	redis.call("DEL",KEYS[1])
	redis.call("DEL",kEYS[2])
	redis.call("DEL",KEYS[3])
	redis.call("DEL",KEYS[4])
	redis.call("DEL",KEYS[5])
	redis.call("DEL",KEYS[6])
	return nil
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`)

	checkBloom = gredis.NewScript(`if(redis.call("EXISTS",KEYS[7])==0)
then
	return nil
end
local r1=redis.call("GETBIT",KEYS[1],ARGV[1])
local r2=redis.call("GETBIT",KEYS[2],ARGV[2])
local r3=redis.call("GETBIT",KEYS[3],ARGV[3])
local r4=redis.call("GETBIT",KEYS[4],ARGV[4])
local r5=redis.call("GETBIT",KEYS[5],ARGV[5])
local r6=redis.call("GETBIT",KEYS[6],ARGV[6])
if(redis.call("EXISTS",KEYS[7])==0)
then
	redis.call("DEL",KEYS[1])
	redis.call("DEL",KEYS[2])
	redis.call("DEL",KEYS[3])
	redis.call("DEL",KEYS[4])
	redis.call("DEL",KEYS[5])
	redis.call("DEL",KEYS[6])
	return nil
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`)
}

// NewBloom -
// groupnum: how many bitset key will be used in redis for this bloom
//
//	every special bitset will have a name like bloomname_[0,groupnum)
//	in cluster mode:this is useful to balance all redis nodes' request
//	in slave mode:set it to 1
//
// bitnum: the capacity of this bloom's each bitset key
//
//	min is 1024
//
// expire: how long will this bloom exist,unit second
//
//	<=0 means no expire
func (c *Client) NewBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, expiresecond int) error {
	if bloomname == "" || groupnum == 0 {
		panic("[redis.bloom.new] bloom name or group num missing")
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	//init memory in redis
	eg := egroup.GetGroup(ctx)
	for i := uint64(0); i < groupnum; i++ {
		index := i
		eg.Go(func(gctx context.Context) error {
			key := bloomname + "_" + strconv.FormatUint(index, 10)
			keybkdr := "{" + key + "}_bkdr"
			keydjb := "{" + key + "}_djb"
			keyfnv := "{" + key + "}_fnv"
			keydek := "{" + key + "}_dek"
			keyrs := "{" + key + "}_rs"
			keysdbm := "{" + key + "}_sdbm"
			keyexist := "{" + key + "}_exist"
			_, e := initBloom.Run(ctx, c, []string{keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist}, bitnum, expiresecond).Int()
			return e
		})
	}
	return egroup.PutGroup(eg)
}

// SetBloom add key into the bloom
// true,this key is not in this bloom and add success
// false,this key maybe already in this bloom,can't 100% confirm
// error: go-redis.Nil means the bloom not exist
func (c *Client) SetBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, userkey string) (bool, error) {
	if bloomname == "" || groupnum == 0 {
		panic("[redis.bloom.set] bloom name or group num missing")
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	key := bloomname + "_" + strconv.FormatUint(common.Bkdrhash(common.STB(userkey), uint64(groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.Bkdrhash(common.STB(userkey), bitnum)
	bit2 := common.Djbhash(common.STB(userkey), bitnum)
	bit3 := common.Fnvhash(common.STB(userkey), bitnum)
	bit4 := common.Dekhash(common.STB(userkey), bitnum)
	bit5 := common.Rshash(common.STB(userkey), bitnum)
	bit6 := common.Sdbmhash(common.STB(userkey), bitnum)
	r, e := setBloom.Run(ctx, c, []string{keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist}, bit1, bit2, bit3, bit4, bit5, bit6).Int()
	if e != nil {
		return false, e
	}
	return r == 0, nil
}

// Check -
// true,this key 100% not in this bloom
// false,this key maybe in this bloom,can't 100% confirm
// error: go-redis.Nil means the bloom not exist
func (c *Client) CheckBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, userkey string) (bool, error) {
	if bloomname == "" || groupnum == 0 {
		panic("[redis.bloom.check] bloom name or group num missing")
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	key := bloomname + "_" + strconv.FormatUint(common.Bkdrhash(common.STB(userkey), uint64(groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.Bkdrhash(common.STB(userkey), bitnum)
	bit2 := common.Djbhash(common.STB(userkey), bitnum)
	bit3 := common.Fnvhash(common.STB(userkey), bitnum)
	bit4 := common.Dekhash(common.STB(userkey), bitnum)
	bit5 := common.Rshash(common.STB(userkey), bitnum)
	bit6 := common.Sdbmhash(common.STB(userkey), bitnum)
	r, e := checkBloom.Run(ctx, c, []string{keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist}, bit1, bit2, bit3, bit4, bit5, bit6).Int()
	if e != nil {
		return false, e
	}
	return r == 0, nil
}
func (c *Client) DelBloom(ctx context.Context, bloomname string, groupnum uint64) error {
	if bloomname == "" || groupnum == 0 {
		panic("[redis.bloom.del] bloom name or group num missing")
	}
	eg := egroup.GetGroup(ctx)
	for i := uint64(0); i < groupnum; i++ {
		index := i
		eg.Go(func(gctx context.Context) error {
			key := bloomname + "_" + strconv.FormatUint(index, 10)
			keybkdr := "{" + key + "}_bkdr"
			keydjb := "{" + key + "}_djb"
			keyfnv := "{" + key + "}_fnv"
			keydek := "{" + key + "}_dek"
			keyrs := "{" + key + "}_rs"
			keysdbm := "{" + key + "}_sdbm"
			keyexist := "{" + key + "}_exist"
			_, e := c.Del(ctx, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist).Result()
			return e
		})
	}
	return egroup.PutGroup(eg)
}
