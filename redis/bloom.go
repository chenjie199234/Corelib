package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gomodule/redigo/redis"
)

func init() {
	h := sha1.Sum([]byte(setbloom))
	hsetbloom = hex.EncodeToString(h[:])
	h = sha1.Sum([]byte(checkbloom))
	hcheckbloom = hex.EncodeToString(h[:])
}

var ErrBloomExpired = errors.New("bloom expired")
var ErrBloomMissingName = errors.New("bloom missing name")
var ErrBloomMissingGroup = errors.New("bloom missing group")

var initbloom = `if(redis.call("EXISTS",KEYS[7])==0)
then
	redis.call("SETBIT",KEYS[1],ARGV[1],1)
	redis.call("SETBIT",KEYS[2],ARGV[2],1)
	redis.call("SETBIT",KEYS[3],ARGV[3],1)
	redis.call("SETBIT",KEYS[4],ARGV[4],1)
	redis.call("SETBIT",KEYS[5],ARGV[5],1)
	redis.call("SETBIT",KEYS[6],ARGV[6],1)
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
end`

// NewBloom -
// groupnum: how many bitset key will be used in redis for this bloom
//
//	every special bitset will have a name like bloomname_[0,groupnum)
//	in cluster mode:this is useful to balance all redis nodes' request
//	in slave mode:set it to 1
//
// bitnum decide the capacity of this bloom's each bitset key
//
//	min is 1024
//
// expire decide how long will this bloom exist
//
//	<=0 means no expire
func (p *Pool) NewBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, expire time.Duration) error {
	if bloomname == "" {
		return ErrBloomMissingName
	}
	if groupnum == 0 {
		return ErrBloomMissingGroup
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	//init memory in redis
	ch := make(chan error, groupnum)
	for i := uint64(0); i < groupnum; i++ {
		tempindex := i
		go func() {
			key := bloomname + "_" + strconv.FormatUint(tempindex, 10)
			keybkdr := "{" + key + "}_bkdr"
			keydjb := "{" + key + "}_djb"
			keyfnv := "{" + key + "}_fnv"
			keydek := "{" + key + "}_dek"
			keyrs := "{" + key + "}_rs"
			keysdbm := "{" + key + "}_sdbm"
			keyexist := "{" + key + "}_exist"
			c, e := p.p.GetContext(ctx)
			if e != nil {
				ch <- e
				return
			}
			defer c.Close()
			_, e = c.(redis.ConnWithContext).DoContext(ctx, "EVAL", initbloom, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bitnum, int64(expire.Seconds()))
			if e != nil && e != redis.ErrNil {
				ch <- e
				return
			}
			ch <- nil
		}()
	}
	for i := uint64(0); i < groupnum; i++ {
		if e := <-ch; e != nil {
			return e
		}
	}
	return nil
}

const setbloom = `if(redis.call("EXISTS",KEYS[7])==0)
then
	return -1
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
	return -1
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`

var hsetbloom string

// SetBloom add key into the bloom
// true,this key is not in this bloom and add success
// false,this key maybe already in this bloom,can't 100% confirm
func (p *Pool) SetBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, userkey string) (bool, error) {
	if bloomname == "" {
		return false, ErrBloomMissingName
	}
	if groupnum == 0 {
		return false, ErrBloomMissingGroup
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	key := bloomname + "_" + strconv.FormatUint(common.BkdrhashString(userkey, uint64(groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.BkdrhashString(key, bitnum)
	bit2 := common.DjbhashString(key, bitnum)
	bit3 := common.FnvhashString(key, bitnum)
	bit4 := common.DekhashString(key, bitnum)
	bit5 := common.RshashString(key, bitnum)
	bit6 := common.SdbmhashString(key, bitnum)
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	cctx := c.(redis.ConnWithContext)
	r, e := redis.Int(cctx.DoContext(ctx, "EVALSHA", hsetbloom, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	if e != nil && strings.Contains(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(cctx.DoContext(ctx, "EVAL", setbloom, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, ErrBloomExpired
	}
	return r == 0, nil
}

var checkbloom = `if(redis.call("EXISTS",KEYS[7])==0)
then
	return -1
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
	return -1
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`

var hcheckbloom string

// Check -
// true,this key 100% not in this bloom
// false,this key maybe in this bloom,can't 100% confirm
func (p *Pool) CheckBloom(ctx context.Context, bloomname string, groupnum uint64, bitnum uint64, userkey string) (bool, error) {
	if bloomname == "" {
		return false, ErrBloomMissingName
	}
	if groupnum == 0 {
		return false, ErrBloomMissingGroup
	}
	if bitnum < 1024 {
		bitnum = 1024
	}
	key := bloomname + "_" + strconv.FormatUint(common.BkdrhashString(userkey, uint64(groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.BkdrhashString(key, bitnum)
	bit2 := common.DjbhashString(key, bitnum)
	bit3 := common.FnvhashString(key, bitnum)
	bit4 := common.DekhashString(key, bitnum)
	bit5 := common.RshashString(key, bitnum)
	bit6 := common.SdbmhashString(key, bitnum)
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	cctx := c.(redis.ConnWithContext)
	r, e := redis.Int(cctx.DoContext(ctx, "EVALSHA", hcheckbloom, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(cctx.DoContext(ctx, "EVAL", checkbloom, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, ErrBloomExpired
	}
	return r == 0, nil
}
