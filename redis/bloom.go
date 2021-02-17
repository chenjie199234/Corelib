package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/common"
	"github.com/gomodule/redigo/redis"
)

type Bloom struct {
	p        *redis.Pool
	bitnum   uint64
	groupnum uint64
	bloomkey string
	expire   int64
}
type BloomConfig struct {
	//useful in cluster mode,to balance all nodes' request
	//every group will have a key bloom[0-Groupnum)
	//default 10
	Groupnum uint64
	//unit is byte
	//single hash list's capacity in every group
	//so this bloom will use memory: Capacity * hash_func_num(6) * Groupnum
	//default 1024*1024(1M)
	Capacity uint64
	//default 30day
	Expire time.Duration
}

func init() {
	//计算setlua脚本的hash值
	h := sha1.Sum([]byte(setlua))
	hsetlua = hex.EncodeToString(h[:])
	//计算check脚本的hash值
	h = sha1.Sum([]byte(checklua))
	hchecklua = hex.EncodeToString(h[:])
}

var existlua = `local e=redis.call("GET",KEYS[1])
if(e)
then
	return e
end
redis.call("SET",KEYS[1],ARGV[1],"EX",ARGV[2])`

var initlua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
if(redis.call("EXISTS",e)==0)
then
local r1=redis.call("SETBIT",k1,ARGV[1],1)
local r2=redis.call("SETBIT",k2,ARGV[1],1)
local r3=redis.call("SETBIT",k3,ARGV[1],1)
local r4=redis.call("SETBIT",k4,ARGV[1],1)
local r5=redis.call("SETBIT",k5,ARGV[1],1)
local r6=redis.call("SETBIT",k6,ARGV[1],1)
redis.call("SET",e,1,"EX",ARGV[2])
redis.call("EXPIRE",k1,ARGV[2])
redis.call("EXPIRE",k2,ARGV[2])
redis.call("EXPIRE",k3,ARGV[2])
redis.call("EXPIRE",k4,ARGV[2])
redis.call("EXPIRE",k5,ARGV[2])
redis.call("EXPIRE",k6,ARGV[2])
end`

// NewRedisBitFilter -
// make sure don't use duplicate bloomkey
func (p *Pool) NewBloom(ctx context.Context, bloomkey string, c *BloomConfig) (*Bloom, error) {
	if p.p == nil {
		return nil, errors.New("redis client not inited")
	}
	if bloomkey == "" {
		return nil, errors.New("bloom key is empty")
	}
	if c.Groupnum == 0 {
		c.Groupnum = 10
	}
	if c.Capacity == 0 {
		c.Capacity = 1024 * 1024
	}
	if c.Expire == 0 {
		c.Expire = 30 * 24 * time.Hour
	}
	//check exist or not
	var exist bool = true
	d, _ := json.Marshal(c)
	conn, e := p.p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	cstr, e := redis.String(conn.Do("EVAL", existlua, 1, bloomkey, d, int64(c.Expire.Seconds())))
	if e != nil {
		if e == redis.ErrNil {
			exist = false
		} else {
			conn.Close()
			return nil, e
		}
	}
	conn.Close()
	//if exist,use exist's config to replace this config
	c = &BloomConfig{}
	if exist {
		if e = json.Unmarshal([]byte(cstr), c); e != nil {
			return nil, errors.New("bloom key conflict")
		}
		if c.Groupnum == 0 || c.Capacity == 0 || c.Expire < time.Hour {
			return nil, errors.New("bloom key conflict")
		}
	}
	instance := &Bloom{
		p:        p.p,
		bitnum:   c.Capacity * 8, //capacity's unit is byte,transform to bit
		groupnum: c.Groupnum,
		expire:   int64(c.Expire.Seconds()),
		bloomkey: bloomkey,
	}
	//init memory in redis
	ch := make(chan error, instance.groupnum)
	for i := uint64(0); i < instance.groupnum; i++ {
		tempindex := i
		go func() {
			key := instance.bloomkey + strconv.FormatUint(tempindex, 10)
			bit := instance.bitnum
			conn, e := p.p.GetContext(ctx)
			if e != nil {
				ch <- e
				return
			}
			defer conn.Close()
			_, e = conn.Do("EVAL", initlua, 1, key, bit, instance.expire)
			if e != nil && e != redis.ErrNil {
				ch <- e
				return
			}
			ch <- nil
		}()
	}
	for i := uint64(0); i < instance.groupnum; i++ {
		if e := <-ch; e != nil {
			return nil, e
		}
	}
	if exist {
		if string(d) == cstr {
			return instance, nil
		}
		return nil, errors.New("bloom key conflict")
	}
	return instance, nil
}

const setlua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
if(redis.call("EXISTS",e)==0)
then
	return -1
end
local r1=redis.call("SETBIT",k1,ARGV[1],1)
local r2=redis.call("SETBIT",k2,ARGV[2],1)
local r3=redis.call("SETBIT",k3,ARGV[3],1)
local r4=redis.call("SETBIT",k4,ARGV[4],1)
local r5=redis.call("SETBIT",k5,ARGV[5],1)
local r6=redis.call("SETBIT",k6,ARGV[6],1)
if(redis.call("EXISTS",e)==0)
then
	redis.call("DEL",k1)
	redis.call("DEL",k2)
	redis.call("DEL",k3)
	redis.call("DEL",k4)
	redis.call("DEL",k5)
	redis.call("DEL",k6)
	return -1
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`

var hsetlua string

// Set add key into the bloom
//true,this key is not in this bloom and add success
//false,this key maybe already in this bloom,can't 100% confirm
func (b *Bloom) Set(ctx context.Context, key string) (bool, error) {
	filterkey := b.getbloomkey(key)
	bit1 := common.BkdrhashString(key, b.bitnum)
	bit2 := common.DjbhashString(key, b.bitnum)
	bit3 := common.FnvhashString(key, b.bitnum)
	bit4 := common.DekhashString(key, b.bitnum)
	bit5 := common.RshashString(key, b.bitnum)
	bit6 := common.SdbmhashString(key, b.bitnum)
	c, e := b.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.Do("EVALSHA", hsetlua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
	if e != nil && e.Error() == "NOSCRIPT No matching script. Please use EVAL." {
		r, e = redis.Int(c.Do("EVAL", setlua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, errors.New("bloom key expired")
	}
	return r == 0, nil
}

var checklua = `local e,k1,k2,k3,k4,k5,k6="{"..KEYS[1].."}e","{"..KEYS[1].."}1","{"..KEYS[1].."}2","{"..KEYS[1].."}3","{"..KEYS[1].."}4","{"..KEYS[1].."}5","{"..KEYS[1].."}6"
if(redis.call("EXISTS",e)==0)
then
	return -1
end
local r1=redis.call("GETBIT",k1,ARGV[1])
local r2=redis.call("GETBIT",k2,ARGV[2])
local r3=redis.call("GETBIT",k3,ARGV[3])
local r4=redis.call("GETBIT",k4,ARGV[4])
local r5=redis.call("GETBIT",k5,ARGV[5])
local r6=redis.call("GETBIT",k6,ARGV[6])
if(redis.call("EXISTS",e)==0)
then
	redis.call("DEL",k1)
	redis.call("DEL",k2)
	redis.call("DEL",k3)
	redis.call("DEL",k4)
	redis.call("DEL",k5)
	redis.call("DEL",k6)
	return -1
end
if(r1==0 or r2==0 or r3==0 or r4==0 or r5==0 or r6==0)
then
	return 0
end
return 1`

var hchecklua string

// Check -
//true,this key 100% not in this bloom
//false,this key maybe in this bloom,can't 100% confirm
func (b *Bloom) Check(ctx context.Context, key string) (bool, error) {
	filterkey := b.getbloomkey(key)
	bit1 := common.BkdrhashString(key, b.bitnum)
	bit2 := common.DjbhashString(key, b.bitnum)
	bit3 := common.FnvhashString(key, b.bitnum)
	bit4 := common.DekhashString(key, b.bitnum)
	bit5 := common.RshashString(key, b.bitnum)
	bit6 := common.SdbmhashString(key, b.bitnum)
	c, e := b.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.Do("EVALSHA", hchecklua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
	if e != nil && e.Error() == "NOSCRIPT No matching script. Please use EVAL." {
		r, e = redis.Int(c.Do("EVAL", checklua, 1, filterkey, bit1, bit2, bit3, bit4, bit5, bit6, b.expire))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, errors.New("bloom key expired")
	}
	return r == 0, nil
}

func (b *Bloom) getbloomkey(userkey string) string {
	return b.bloomkey + strconv.FormatUint(common.BkdrhashString(userkey, uint64(b.groupnum)), 10)
}
