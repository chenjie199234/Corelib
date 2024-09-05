package mids

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/redis"
)

type access struct {
	c     *redis.Client
	grpc  map[string]map[string]string //first key path,second key accessid,value accesskey
	crpc  map[string]map[string]string
	get   map[string]map[string]string
	post  map[string]map[string]string
	put   map[string]map[string]string
	patch map[string]map[string]string
	del   map[string]map[string]string
}
type MultiPathAccessConfigs map[string]SinglePathAccessConfig //map's key:path
type SinglePathAccessConfig []*PathAccessRule                 //one path can have multi access rule
type PathAccessRule struct {
	Methods  []string          `json:"methods"`  //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	Accesses map[string]string `json:"accesses"` //key accessid,value accesskey,all method above share these accesses
}

var accessInstance *access

func init() {
	accessInstance = &access{}
}

// key path
func UpdateAccessConfig(c MultiPathAccessConfigs) {
	grpc := make(map[string]map[string]string)
	crpc := make(map[string]map[string]string)
	get := make(map[string]map[string]string)
	post := make(map[string]map[string]string)
	put := make(map[string]map[string]string)
	patch := make(map[string]map[string]string)
	del := make(map[string]map[string]string)
	for path, pathaccessrules := range c {
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
		for _, pathaccessrule := range pathaccessrules {
			for _, method := range pathaccessrule.Methods {
				switch strings.ToUpper(method) {
				case "GRPC":
					if _, ok := grpc[path]; !ok {
						grpc[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						grpc[path][accessid] = accesskey
					}
				case "CRPC":
					if _, ok := crpc[path]; !ok {
						crpc[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						crpc[path][accessid] = accesskey
					}
				case "GET":
					if _, ok := get[path]; !ok {
						get[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						get[path][accessid] = accesskey
					}
				case "POST":
					if _, ok := post[path]; !ok {
						post[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						post[path][accessid] = accesskey
					}
				case "PUT":
					if _, ok := put[path]; !ok {
						put[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						put[path][accessid] = accesskey
					}
				case "PATCH":
					if _, ok := patch[path]; !ok {
						patch[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						patch[path][accessid] = accesskey
					}
				case "DELETE":
					if _, ok := del[path]; !ok {
						del[path] = make(map[string]string)
					}
					for accessid, accesskey := range pathaccessrule.Accesses {
						del[path][accessid] = accesskey
					}
				}
			}
		}
	}
	accessInstance.grpc = grpc
	accessInstance.crpc = crpc
	accessInstance.get = get
	accessInstance.post = post
	accessInstance.put = put
	accessInstance.patch = patch
	accessInstance.del = del
}
func UpdateReplayDefendRedisInstance(c *redis.Client) {
	if c == nil {
		slog.WarnContext(nil, "[access.sign] redis missing,replay attack may happened")
	}
	oldp := (*redis.Client)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&accessInstance.c)), unsafe.Pointer(c)))
	if oldp != nil {
		oldp.Close()
	}
}

func VerifyAccessKey(ctx context.Context, method, path, accesskey string) bool {
	var tmp map[string]map[string]string
	switch strings.ToUpper(method) {
	case "GRPC":
		tmp = accessInstance.grpc
	case "CRPC":
		tmp = accessInstance.crpc
	case "GET":
		tmp = accessInstance.get
	case "POST":
		tmp = accessInstance.post
	case "PUT":
		tmp = accessInstance.put
	case "PATCH":
		tmp = accessInstance.patch
	case "DELETE":
		tmp = accessInstance.del
	default:
		return false
	}
	if tmp == nil {
		slog.ErrorContext(ctx, "[access.key] missing init,please use UpdateAccessConfig first")
		return false
	}
	accesses, ok := tmp[path]
	if !ok {
		return false
	}
	for _, v := range accesses {
		if accesskey == v {
			return true
		}
	}
	return false
}
