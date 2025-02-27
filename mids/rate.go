package mids

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/redis"
)

type rate struct {
	c     *redis.Client
	grpc  map[string][][3]any //key path
	crpc  map[string][][3]any //key path
	get   map[string][][3]any //key path
	post  map[string][][3]any //key path
	put   map[string][][3]any //key path
	patch map[string][][3]any //key path
	del   map[string][][3]any //key path
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

func UpdateRateRedisInstance(c *redis.Client) {
	if c == nil {
		slog.WarnContext(nil, "[rate] redis missing,all rate check will be failed")
	}
	oldp := (*redis.Client)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.c)), unsafe.Pointer(c)))
	if oldp != nil {
		oldp.Close()
	}
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
			if pathraterule.RateType != "path" && pathraterule.RateType != "token" && pathraterule.RateType != "session" {
				slog.ErrorContext(nil, "[rate] rate config's rate_type must be path/token/session", slog.String("path", path), slog.String("rate_type", pathraterule.RateType))
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
	rateinstance.grpc = grpc
	rateinstance.crpc = crpc
	rateinstance.get = get
	rateinstance.post = post
	rateinstance.put = put
	rateinstance.patch = patch
	rateinstance.del = del
}

func checkrate(ctx context.Context, infos [][3]any) bool {
	redisclient := rateinstance.c
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
			rates[info[0].(string)+"_"+token] = [2]uint64{info[1].(uint64), info[2].(uint64)}
		} else if strings.HasPrefix(info[0].(string), "session_rate") {
			md := metadata.GetMetadata(ctx)
			session, ok := md["Session-User"]
			if !ok {
				slog.ErrorContext(ctx, "[rate] missing session when check session's rate,make sure the session midware is before the rate midware")
				return false
			}
			rates[info[0].(string)+"_"+session] = [2]uint64{info[1].(uint64), info[2].(uint64)}
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

func GrpcRate(ctx context.Context, path string) bool {
	if rateinstance.grpc == nil {
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
		slog.ErrorContext(ctx, "[rate] missing init,please use UpdateRateConfig first")
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
