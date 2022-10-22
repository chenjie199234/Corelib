package mids

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/container/ring"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/redis"
)

type rate struct {
	p     *redis.Pool
	grpc  map[string]*rateinfo //key path
	crpc  map[string]*rateinfo //key path
	get   map[string]*rateinfo //key path
	post  map[string]*rateinfo //key path
	put   map[string]*rateinfo //key path
	patch map[string]*rateinfo //key path
	del   map[string]*rateinfo //key path
}
type rateinfo struct {
	single *ring.Ring[int64]
	global []interface{} //if this is not nil,this has 2 elements,first is the redis key,second is the rate
}

var rateinstance *rate

func init() {
	rateinstance = &rate{}
	str := os.Getenv("RATE_REDIS_URL")
	if str == "" {
		log.Warning(nil, "[rate] env RATE_REDIS_URL missing,all global rate check will be failed")
		return
	}
	rateinstance.p = redis.NewRedis(&redis.Config{
		RedisName:   "rate_redis",
		URL:         str,
		MaxOpen:     0,    //means no limit
		MaxIdle:     1024, //the pool's buf
		MaxIdletime: time.Minute,
		ConnTimeout: time.Second * 5,
		IOTimeout:   time.Second * 5,
	})
}

type RateConfig struct {
	Path   string
	Method []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	//single and global are both 0,means ban
	//single and global are both not 0,means both single and global will be checked
	//single is 0,global not 0,means only global will be checked
	//single it not 0,global is 0,means only single will be checked
	SingleMaxPerSec uint64 //single instance's rate
	GlobalMaxPerSec uint64 //all instances' rate
}

func UpdateRateConfig(c []*RateConfig) {
	grpc := make(map[string]*rateinfo)  //key path
	crpc := make(map[string]*rateinfo)  //key path
	get := make(map[string]*rateinfo)   //key path
	post := make(map[string]*rateinfo)  //key path
	put := make(map[string]*rateinfo)   //key path
	patch := make(map[string]*rateinfo) //key path
	del := make(map[string]*rateinfo)   //key path
	for _, cc := range c {
		info := &rateinfo{}
		if cc.SingleMaxPerSec != 0 {
			info.single = ring.NewRing[int64](cc.SingleMaxPerSec)
		}
		if cc.GlobalMaxPerSec != 0 {
			info.global = []interface{}{cc.Path + "_" + strings.Join(cc.Method, "_"), cc.GlobalMaxPerSec}
		}
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = info
			case "CRPC":
				crpc[cc.Path] = info
			case "GET":
				get[cc.Path] = info
			case "POST":
				post[cc.Path] = info
			case "PUT":
				put[cc.Path] = info
			case "PATCH":
				patch[cc.Path] = info
			case "DELETE":
				del[cc.Path] = info
			}
		}
	}
	rateinstance.grpc = grpc
	rateinstance.crpc = crpc
	rateinstance.get = get
	rateinstance.post = post
	rateinstance.put = put
	rateinstance.patch = patch
	rateinstance.del = del
}

func checkrate(ctx context.Context, info *rateinfo) (bool, error) {
	if info.global == nil && info.single == nil {
		//both single and global's config rate is 0
		return false, nil
	}
	if info.global != nil && rateinstance.p == nil {
		//didn't set the rate redis
		return false, nil
	}
	//single first
	if info.single != nil {
		now := time.Now().UnixNano()
		for {
			if info.single.Push(now) {
				break
			}
			//buf list full,try to pop
			if _, ok := info.single.Pop(func(d int64) bool {
				return now-d >= time.Second.Nanoseconds()
			}); !ok {
				//can't push and can't pop,buf list is still full
				return false, nil
			}
		}
	}
	//then global
	if info.global == nil {
		return true, nil
	}
	pass, e := rateinstance.p.RateLimitSecondMax(ctx, info.global[0].(string), info.global[1].(uint64))
	if !pass && info.single != nil {
		//when pass the single check,current time will be pushed into the buf list
		//now the global check didn't pass,we need to return back the consumed rate
		//but when return back the consumed rate,the oldest try will be poped
		//so this is not fair,only the num can be returned,better then do nothing
		info.single.Pop(nil)
	}
	return pass, e
}

func GrpcRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.grpc == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.grpc[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func CrpcRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.crpc == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.crpc[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func HttpGetRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.get == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.get[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func HttpPostRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.post == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.post[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func HttpPutRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.put == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.put[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func HttpPatchRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.patch == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.patch[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
func HttpDelRate(ctx context.Context, path string) (bool, error) {
	if rateinstance.del == nil {
		//didn't update the config
		return false, nil
	}
	info, ok := rateinstance.del[path]
	if !ok {
		//missing config
		return false, nil
	}
	return checkrate(ctx, info)
}
