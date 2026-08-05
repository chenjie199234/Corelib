package mids

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/redis"
)

type rate struct {
	c     atomic.Pointer[redis.Client]
	grpc  atomic.Pointer[map[string][][3]any] //key path
	crpc  atomic.Pointer[map[string][][3]any] //key path
	get   atomic.Pointer[map[string][][3]any] //key path
	post  atomic.Pointer[map[string][][3]any] //key path
	put   atomic.Pointer[map[string][][3]any] //key path
	patch atomic.Pointer[map[string][][3]any] //key path
	del   atomic.Pointer[map[string][][3]any] //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{}
}

type MultiPathRateConfigs map[string]SinglePathRateConfig //map's key:path
type SinglePathRateConfig []*PathRateRule                 // one path can have multi rate rules
type PathRateRule struct {
	Methods []string `json:"methods"` //CRPC,GRPC,GET,POST,PUT,PATCH,DELETE
	//MaxRate per Period(uint second)
	MaxRate  uint64 `json:"max_rate"`  //all methods above share this rate
	Period   uint64 `json:"period"`    //uint second
	RateType string `json:"rate_type"` //path,token,session
}

func UpdateRateRedisInstance(c *redis.Client) (old *redis.Client) {
	if c == nil {
		slog.Warn("[rate] redis missing,all rate check will be failed")
	}
	return rateinstance.c.Swap(c)
}

// key path
func UpdateRateConfig(c MultiPathRateConfigs) {
	grpc := make(map[string][][3]any)  //key path
	crpc := make(map[string][][3]any)  //key path
	get := make(map[string][][3]any)   //key path
	post := make(map[string][][3]any)  //key path
	put := make(map[string][][3]any)   //key path
	patch := make(map[string][][3]any) //key path
	del := make(map[string][][3]any)   //key path
	for path, pathraterules := range c {
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
		for _, pathraterule := range pathraterules {
			if pathraterule == nil {
				continue
			}
			if pathraterule.RateType != "path" && pathraterule.RateType != "token" && pathraterule.RateType != "session" {
				slog.Error("[rate] rate config's rate_type must be path/token/session", slog.String("path", path), slog.String("rate_type", pathraterule.RateType))
				return
			}

			var rateinfo [3]any
			rateinfo[0] = pathraterule.RateType + "_rate_{" + path + "}_" + strings.Join(pathraterule.Methods, "_") + "_" + strconv.FormatUint(pathraterule.Period, 10)
			rateinfo[1] = pathraterule.MaxRate
			rateinfo[2] = pathraterule.Period
			for _, m := range pathraterule.Methods {
				switch strings.ToUpper(m) {
				case "GRPC":
					if _, ok := grpc[path]; !ok {
						grpc[path] = make([][3]any, 0, 3)
					}
					grpc[path] = append(grpc[path], rateinfo)
				case "CRPC":
					if _, ok := crpc[path]; !ok {
						crpc[path] = make([][3]any, 0, 3)
					}
					crpc[path] = append(crpc[path], rateinfo)
				case "GET":
					if _, ok := get[path]; !ok {
						get[path] = make([][3]any, 0, 3)
					}
					get[path] = append(get[path], rateinfo)
				case "POST":
					if _, ok := post[path]; !ok {
						post[path] = make([][3]any, 0, 3)
					}
					post[path] = append(post[path], rateinfo)
				case "PUT":
					if _, ok := put[path]; !ok {
						put[path] = make([][3]any, 0, 3)
					}
					put[path] = append(put[path], rateinfo)
				case "PATCH":
					if _, ok := patch[path]; !ok {
						patch[path] = make([][3]any, 0, 3)
					}
					patch[path] = append(patch[path], rateinfo)
				case "DELETE":
					if _, ok := del[path]; !ok {
						del[path] = make([][3]any, 0, 3)
					}
					del[path] = append(del[path], rateinfo)
				}
			}
		}
	}
	rateinstance.grpc.Store(&grpc)
	rateinstance.crpc.Store(&crpc)
	rateinstance.get.Store(&get)
	rateinstance.post.Store(&post)
	rateinstance.put.Store(&put)
	rateinstance.patch.Store(&patch)
	rateinstance.del.Store(&del)
}

func checkrate(ctx context.Context, infos [][3]any) bool {
	redisclient := rateinstance.c.Load()
	if redisclient == nil {
		slog.ErrorContext(ctx, "[rate] redis missing")
		return false
	}
	rates := make(map[string][2]uint64)
	for _, info := range infos {
		if strings.HasPrefix(info[0].(string), "token_rate") {
			md := metadata.GetMetadata(ctx)
			token, ok := md["Token-User"]
			if !ok {
				slog.ErrorContext(ctx, "[rate] missing token when check token's rate,make sure the token midware is before the rate midware")
				return false
			}
			if exist, ok := rates[info[0].(string)+"_"+token]; ok {
				//same rule on the path method and period,use the smaller one
				if exist[0] > info[1].(uint64) {
					rates[info[0].(string)+"_"+token] = [2]uint64{info[1].(uint64), info[2].(uint64)}
				}
			} else {
				rates[info[0].(string)+"_"+token] = [2]uint64{info[1].(uint64), info[2].(uint64)}
			}
		} else if strings.HasPrefix(info[0].(string), "session_rate") {
			md := metadata.GetMetadata(ctx)
			session, ok := md["Session-User"]
			if !ok {
				slog.ErrorContext(ctx, "[rate] missing session when check session's rate,make sure the session midware is before the rate midware")
				return false
			}
			if exist, ok := rates[info[0].(string)+"_"+session]; ok {
				//same rule on the path method and period,use the smaller one
				if exist[0] > info[1].(uint64) {
					rates[info[0].(string)+"_"+session] = [2]uint64{info[1].(uint64), info[2].(uint64)}
				}
			} else {
				rates[info[0].(string)+"_"+session] = [2]uint64{info[1].(uint64), info[2].(uint64)}
			}
		} else if exist, ok := rates[info[0].(string)]; ok {
			//same rule on the path method and period,use the smaller one
			if exist[0] > info[1].(uint64) {
				rates[info[0].(string)] = [2]uint64{info[1].(uint64), info[2].(uint64)}
			}
		} else {
			rates[info[0].(string)] = [2]uint64{info[1].(uint64), info[2].(uint64)}
		}
	}
	pass, e := redisclient.RateLimit(ctx, rates)
	if e != nil {
		slog.ErrorContext(ctx, "[rate] redis op failed", slog.String("error", e.Error()))
	}
	return pass
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func GrpcRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.grpc.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func CrpcRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.crpc.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func HttpGetRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.get.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func HttpPostRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.post.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func HttpPutRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.put.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func HttpPatchRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.patch.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}

// call this func means this path must pass the rate check
// missing config of this path means can't pass the check
func HttpDelRate(ctx context.Context, path string) (pass bool) {
	tmp := rateinstance.del.Load()
	if tmp == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
		//didn't update the config
		return false
	}
	infos, ok := (*tmp)[path]
	if !ok {
		//this path need rate limiter,but we can't find the rate config
		return false
	}
	return checkrate(ctx, infos)
}
