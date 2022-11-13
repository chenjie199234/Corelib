package mids

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/redis"
)

type rate struct {
	p     *redis.Pool
	grpc  map[string][][2]interface{} //key path
	crpc  map[string][][2]interface{} //key path
	get   map[string][][2]interface{} //key path
	post  map[string][][2]interface{} //key path
	put   map[string][][2]interface{} //key path
	patch map[string][][2]interface{} //key path
	del   map[string][][2]interface{} //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{}
}

type PathRateConfig struct {
	Method    []string `json:"method"`      //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec uint64   `json:"max_per_sec"` //all methods above share this rate
}

func UpdateRateRedisUrl(redisurl string) {
	var newp *redis.Pool
	if redisurl != "" {
		newp = redis.NewRedis(&redis.Config{
			RedisName:   "rate_redis",
			URL:         redisurl,
			MaxOpen:     0,    //means no limit
			MaxIdle:     1024, //the pool's buf
			MaxIdletime: time.Minute,
			ConnTimeout: time.Second * 5,
			IOTimeout:   time.Second * 5,
		})
	} else {
		log.Warning(nil, "[rate] redis missing,all rate check will be failed")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.p)), unsafe.Pointer(newp)))
	if oldp != nil {
		oldp.Close()
	}
}
func UpdateRateRedisInstance(p *redis.Pool) {
	if p == nil {
		log.Warning(nil, "[rate] redis missing,all rate check will be failed")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.p)), unsafe.Pointer(p)))
	if oldp != nil {
		oldp.Close()
	}
}

// key path
func UpdateRateConfig(c map[string][]*PathRateConfig) {
	grpc := make(map[string][][2]interface{})  //key path
	crpc := make(map[string][][2]interface{})  //key path
	get := make(map[string][][2]interface{})   //key path
	post := make(map[string][][2]interface{})  //key path
	put := make(map[string][][2]interface{})   //key path
	patch := make(map[string][][2]interface{}) //key path
	del := make(map[string][][2]interface{})   //key path
	for path, cc := range c {
		for _, ccc := range cc {
			var rateinfo [2]interface{}
			rateinfo[0] = "{" + path + "}_" + strings.Join(ccc.Method, "_")
			rateinfo[1] = ccc.MaxPerSec
			for _, m := range ccc.Method {
				switch strings.ToUpper(m) {
				case "GRPC":
					if _, ok := grpc[path]; !ok {
						grpc[path] = make([][2]interface{}, 0, 3)
					}
					grpc[path] = append(grpc[path], rateinfo)
				case "CRPC":
					if _, ok := crpc[path]; !ok {
						crpc[path] = make([][2]interface{}, 0, 3)
					}
					crpc[path] = append(crpc[path], rateinfo)
				case "GET":
					if _, ok := get[path]; !ok {
						get[path] = make([][2]interface{}, 0, 3)
					}
					get[path] = append(get[path], rateinfo)
				case "POST":
					if _, ok := post[path]; !ok {
						post[path] = make([][2]interface{}, 0, 3)
					}
					post[path] = append(post[path], rateinfo)
				case "PUT":
					if _, ok := put[path]; !ok {
						put[path] = make([][2]interface{}, 0, 3)
					}
					put[path] = append(put[path], rateinfo)
				case "PATCH":
					if _, ok := patch[path]; !ok {
						patch[path] = make([][2]interface{}, 0, 3)
					}
					patch[path] = append(patch[path], rateinfo)
				case "DELETE":
					if _, ok := del[path]; !ok {
						del[path] = make([][2]interface{}, 0, 3)
					}
					del[path] = append(del[path], rateinfo)
				}
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

func checkrate(ctx context.Context, infos [][2]interface{}) bool {
	redisclient := rateinstance.p
	if redisclient == nil {
		log.Error(ctx, "[rate] config missing redis")
		return false
	}
	rates := make(map[string]uint64)
	for _, info := range infos {
		rates[info[0].(string)] = info[1].(uint64)
	}
	pass, e := redisclient.RateLimitSecondMax(ctx, rates)
	if e != nil {
		log.Error(ctx, "[rate] update redis check data:", e)
	}
	return pass
}

func GrpcRate(ctx context.Context, path string) bool {
	if rateinstance.grpc == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.grpc[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func CrpcRate(ctx context.Context, path string) bool {
	if rateinstance.crpc == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.crpc[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func HttpGetRate(ctx context.Context, path string) bool {
	if rateinstance.get == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.get[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func HttpPostRate(ctx context.Context, path string) bool {
	if rateinstance.post == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.post[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func HttpPutRate(ctx context.Context, path string) bool {
	if rateinstance.put == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.put[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func HttpPatchRate(ctx context.Context, path string) bool {
	if rateinstance.patch == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.patch[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
func HttpDelRate(ctx context.Context, path string) bool {
	if rateinstance.del == nil {
		log.Error(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := rateinstance.del[path]
	if !ok {
		//missing config
		return false
	}
	return checkrate(ctx, infos)
}
