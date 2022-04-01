package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/gomodule/redigo/redis"
)

type Bloom struct {
	p *Pool
	c *BloomConfig
}

type BloomConfig struct {
	// make sure don't use duplicate BloomName between different blooms
	BloomName string
	//Groupnum decide there are how many keys will be used in redis for this bloom
	//this is useful to balance all redis nodes' request
	//every special key will have a name like BloomName_[0,Groupnum)
	Groupnum uint64 //default 10
	//unit is bit
	//single hash list's capacity in every group
	//so this bloom will use memory: Capacity / 8 * hash_func_num(6) * Groupnum
	Capacity uint64        //default 1024*1024*8(1M)
	Expire   time.Duration //default 30day,min 1hour
}

func (c *BloomConfig) validate() error {
	if c == nil {
		return errors.New("config is empty")
	}
	if c.BloomName == "" {
		return errors.New("BloomName is empty")
	}
	if c.Groupnum == 0 {
		c.Groupnum = 10
	}
	if c.Capacity == 0 {
		c.Capacity = 1024 * 1024 * 8
	}
	if c.Expire == 0 {
		c.Expire = 30 * 24 * time.Hour
	} else if c.Expire < time.Hour {
		c.Expire = time.Hour
	}
	return nil
}

func init() {
	h := sha1.Sum([]byte(setlua))
	hsetlua = hex.EncodeToString(h[:])
	h = sha1.Sum([]byte(checklua))
	hchecklua = hex.EncodeToString(h[:])
}

var existlua = `local e=redis.call("GET",KEYS[1])
if(e)
then
	return e
end
redis.call("SET",KEYS[1],ARGV[1],"EX",ARGV[2])`

var initlua = `if(redis.call("EXISTS",KEYS[7])==0)
then
local r1=redis.call("SETBIT",KEYS[1],ARGV[1],1)
local r2=redis.call("SETBIT",KEYS[2],ARGV[1],1)
local r3=redis.call("SETBIT",KEYS[3],ARGV[1],1)
local r4=redis.call("SETBIT",KEYS[4],ARGV[1],1)
local r5=redis.call("SETBIT",KEYS[5],ARGV[1],1)
local r6=redis.call("SETBIT",KEYS[6],ARGV[1],1)
redis.call("SET",KEYS[7],1,"EX",ARGV[2])
redis.call("EXPIRE",KEYS[1],ARGV[2])
redis.call("EXPIRE",KEYS[2],ARGV[2])
redis.call("EXPIRE",KEYS[3],ARGV[2])
redis.call("EXPIRE",KEYS[4],ARGV[2])
redis.call("EXPIRE",KEYS[5],ARGV[2])
redis.call("EXPIRE",KEYS[6],ARGV[2])
end`

var ErrBloomConflict = errors.New("bloom conflict")
var ErrBloomExpired = errors.New("bloom expired")

// NewRedisBitFilter -
func (p *Pool) NewBloom(ctx context.Context, c *BloomConfig) (*Bloom, error) {
	if e := c.validate(); e != nil {
		return nil, e
	}
	//check exist or not
	var exist bool = true
	d, _ := json.Marshal(c)
	conn, e := p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	cstr, e := redis.String(conn.DoContext(ctx, "EVAL", existlua, 1, c.BloomName, d, int64(c.Expire.Seconds())))
	if e != nil {
		if e == redis.ErrNil {
			exist = false
		} else {
			return nil, e
		}
	}
	if exist {
		//if exist,use exist's config to replace this config
		c = &BloomConfig{}
		if e = json.Unmarshal(common.Str2byte(cstr), c); e != nil {
			return nil, ErrBloomConflict
		}
		if c.Groupnum == 0 || c.Capacity == 0 || c.Expire < time.Hour {
			return nil, ErrBloomConflict
		}
	}
	instance := &Bloom{
		p: p,
		c: c,
	}
	//init memory in redis
	ch := make(chan error, c.Groupnum)
	for i := uint64(0); i < c.Groupnum; i++ {
		tempindex := i
		go func() {
			key := c.BloomName + "_" + strconv.FormatUint(tempindex, 10)
			keybkdr := "{" + key + "}_bkdr"
			keydjb := "{" + key + "}_djb"
			keyfnv := "{" + key + "}_fnv"
			keydek := "{" + key + "}_dek"
			keyrs := "{" + key + "}_rs"
			keysdbm := "{" + key + "}_sdbm"
			keyexist := "{" + key + "}_exist"
			conn, e := p.GetContext(ctx)
			if e != nil {
				ch <- e
				return
			}
			defer conn.Close()
			_, e = conn.DoContext(ctx, "EVAL", initlua, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, c.Capacity, int64(c.Expire.Seconds()))
			if e != nil && e != redis.ErrNil {
				ch <- e
				return
			}
			ch <- nil
		}()
	}
	for i := uint64(0); i < c.Groupnum; i++ {
		if e := <-ch; e != nil {
			return nil, e
		}
	}
	if exist {
		if string(d) == cstr {
			return instance, nil
		}
		return nil, ErrBloomConflict
	}
	return instance, nil
}

const setlua = `if(redis.call("EXISTS",KEYS[7])==0)
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

var hsetlua string

// Set add key into the bloom
//true,this key is not in this bloom and add success
//false,this key maybe already in this bloom,can't 100% confirm
func (b *Bloom) Set(ctx context.Context, userkey string) (bool, error) {
	key := b.c.BloomName + "_" + strconv.FormatUint(common.BkdrhashString(userkey, uint64(b.c.Groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.BkdrhashString(key, b.c.Capacity)
	bit2 := common.DjbhashString(key, b.c.Capacity)
	bit3 := common.FnvhashString(key, b.c.Capacity)
	bit4 := common.DekhashString(key, b.c.Capacity)
	bit5 := common.RshashString(key, b.c.Capacity)
	bit6 := common.SdbmhashString(key, b.c.Capacity)
	c, e := b.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.DoContext(ctx, "EVALSHA", hsetlua, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	if e != nil && strings.Contains(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(c.DoContext(ctx, "EVAL", setlua, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, ErrBloomExpired
	}
	return r == 0, nil
}

var checklua = `if(redis.call("EXISTS",KEYS[7])==0)
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

var hchecklua string

// Check -
//true,this key 100% not in this bloom
//false,this key maybe in this bloom,can't 100% confirm
func (b *Bloom) Check(ctx context.Context, userkey string) (bool, error) {
	key := b.c.BloomName + "_" + strconv.FormatUint(common.BkdrhashString(userkey, uint64(b.c.Groupnum)), 10)
	keybkdr := "{" + key + "}_bkdr"
	keydjb := "{" + key + "}_djb"
	keyfnv := "{" + key + "}_fnv"
	keydek := "{" + key + "}_dek"
	keyrs := "{" + key + "}_rs"
	keysdbm := "{" + key + "}_sdbm"
	keyexist := "{" + key + "}_exist"
	bit1 := common.BkdrhashString(key, b.c.Capacity)
	bit2 := common.DjbhashString(key, b.c.Capacity)
	bit3 := common.FnvhashString(key, b.c.Capacity)
	bit4 := common.DekhashString(key, b.c.Capacity)
	bit5 := common.RshashString(key, b.c.Capacity)
	bit6 := common.SdbmhashString(key, b.c.Capacity)
	c, e := b.p.GetContext(ctx)
	if e != nil {
		return false, e
	}
	defer c.Close()
	r, e := redis.Int(c.DoContext(ctx, "EVALSHA", hchecklua, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	if e != nil && strings.HasPrefix(e.Error(), "NOSCRIPT") {
		r, e = redis.Int(c.DoContext(ctx, "EVAL", checklua, 7, keybkdr, keydjb, keyfnv, keydek, keyrs, keysdbm, keyexist, bit1, bit2, bit3, bit4, bit5, bit6))
	}
	if e != nil {
		return false, e
	}
	if r == -1 {
		return false, ErrBloomExpired
	}
	return r == 0, nil
}
