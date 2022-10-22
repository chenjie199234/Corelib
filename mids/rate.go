package mids

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/container/ring"
	"github.com/chenjie199234/Corelib/redis"
)

type selfrate struct {
	grpc  map[string]*ring.Ring[int64] //key path
	crpc  map[string]*ring.Ring[int64] //key path
	get   map[string]*ring.Ring[int64] //key path
	post  map[string]*ring.Ring[int64] //key path
	put   map[string]*ring.Ring[int64] //key path
	patch map[string]*ring.Ring[int64] //key path
	del   map[string]*ring.Ring[int64] //key path
}
type globalrate struct {
	p     *redis.Pool
	grpc  map[string][]interface{} //key path,value has 2 elements,first is the redis key,second is the max rate per second
	crpc  map[string][]interface{}
	get   map[string][]interface{}
	post  map[string][]interface{}
	put   map[string][]interface{}
	patch map[string][]interface{}
	del   map[string][]interface{}
}

var selfrateinstance *selfrate
var globalrateinstance *globalrate

func init() {
	selfrateinstance = &selfrate{}
	globalrateinstance = &globalrate{}
}

type RateConfig struct {
	Path      string
	Method    []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec uint64   //0 means ban
}

func UpdateSelfRateConfig(c []*RateConfig) {
	grpc := make(map[string]*ring.Ring[int64])  //key path
	crpc := make(map[string]*ring.Ring[int64])  //key path
	get := make(map[string]*ring.Ring[int64])   //key path
	post := make(map[string]*ring.Ring[int64])  //key path
	put := make(map[string]*ring.Ring[int64])   //key path
	patch := make(map[string]*ring.Ring[int64]) //key path
	del := make(map[string]*ring.Ring[int64])   //key path
	for _, cc := range c {
		r := ring.NewRing[int64](cc.MaxPerSec)
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = r
			case "CRPC":
				crpc[cc.Path] = r
			case "GET":
				get[cc.Path] = r
			case "POST":
				post[cc.Path] = r
			case "PUT":
				put[cc.Path] = r
			case "PATCH":
				patch[cc.Path] = r
			case "DELETE":
				del[cc.Path] = r
			}
		}
	}
	selfrateinstance.grpc = grpc
	selfrateinstance.crpc = crpc
	selfrateinstance.get = get
	selfrateinstance.post = post
	selfrateinstance.put = put
	selfrateinstance.patch = patch
	selfrateinstance.del = del
}
func UpdateGlobalRateConfig(redisc *redis.Config, ratec []*RateConfig) {
	p := redis.NewRedis(redisc)
	grpc := make(map[string][]interface{})
	crpc := make(map[string][]interface{})
	get := make(map[string][]interface{})
	post := make(map[string][]interface{})
	put := make(map[string][]interface{})
	patch := make(map[string][]interface{})
	del := make(map[string][]interface{})
	for _, c := range ratec {
		rediskey := c.Path + "_" + strings.Join(c.Method, "_")
		for _, m := range c.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "CRPC":
				crpc[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "GET":
				get[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "POST":
				post[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "PUT":
				put[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "PATCH":
				patch[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			case "DELETE":
				del[c.Path] = []interface{}{rediskey, c.MaxPerSec}
			}
		}
	}
	if old := atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&globalrateinstance.p)), unsafe.Pointer(p)); old != nil && (*redis.Pool)(old) != nil {
		(*redis.Pool)(old).Close()
	}
	globalrateinstance.grpc = grpc
	globalrateinstance.crpc = crpc
	globalrateinstance.get = get
	globalrateinstance.post = post
	globalrateinstance.put = put
	globalrateinstance.patch = patch
	globalrateinstance.del = del
}

func checkrate(buf *ring.Ring[int64]) bool {
	now := time.Now().UnixNano()
	for {
		if buf.Push(now) {
			return true
		}
		//buf full,try to pop
		if _, ok := buf.Pop(func(d int64) bool {
			if now-d >= time.Second.Nanoseconds() {
				return true
			}
			return false
		}); !ok {
			//can't push and can't pop,buf is still full
			break
		}
	}
	return false
}

func GrpcRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.grpc == nil {
			return false, nil
		}
		params, ok := globalrateinstance.grpc[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.grpc == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.grpc[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func CrpcRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.crpc == nil {
			return false, nil
		}
		params, ok := globalrateinstance.crpc[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.crpc == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.crpc[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func HttpGetRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.get == nil {
			return false, nil
		}
		params, ok := globalrateinstance.get[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.get == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.get[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func HttpPostRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.post == nil {
			return false, nil
		}
		params, ok := globalrateinstance.post[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.post == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.post[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func HttpPutRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.put == nil {
			return false, nil
		}
		params, ok := globalrateinstance.put[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.put == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.put[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func HttpPatchRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.patch == nil {
			return false, nil
		}
		params, ok := globalrateinstance.patch[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.patch == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.patch[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
func HttpDelRate(ctx context.Context, path string, global bool) (bool, error) {
	if global {
		if globalrateinstance.p == nil || globalrateinstance.del == nil {
			return false, nil
		}
		params, ok := globalrateinstance.del[path]
		if !ok {
			return false, nil
		}
		return globalrateinstance.p.RateLimitSecondMax(ctx, params[0].(string), params[1].(uint64))
	} else {
		if selfrateinstance.del == nil {
			return false, nil
		}
		buf, ok := selfrateinstance.del[path]
		if !ok {
			return true, nil
		}
		return checkrate(buf), nil
	}
}
