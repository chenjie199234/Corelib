package mids

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
)

// key path,value accesskey
var grpcAccess atomic.Pointer[map[string][]string]
var crpcAccess atomic.Pointer[map[string][]string]
var getAccess atomic.Pointer[map[string][]string]
var postAccess atomic.Pointer[map[string][]string]
var putAccess atomic.Pointer[map[string][]string]
var patchAccess atomic.Pointer[map[string][]string]
var delAccess atomic.Pointer[map[string][]string]

type MultiPathAccessConfigs map[string]SinglePathAccessConfig //map's key:path
type SinglePathAccessConfig []*PathAccessRule                 //one path can have multi access rule
type PathAccessRule struct {
	Methods  []string `json:"methods"`  //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	Accesses []string `json:"accesses"` //value accesskey,all method above share these accesses
}

// key path
func UpdateAccessConfig(c MultiPathAccessConfigs) {
	tmpgrpc := make(map[string][]string)
	tmpcrpc := make(map[string][]string)
	tmpget := make(map[string][]string)
	tmppost := make(map[string][]string)
	tmpput := make(map[string][]string)
	tmppatch := make(map[string][]string)
	tmpdel := make(map[string][]string)
	for path, pathaccessrules := range c {
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
		for _, pathaccessrule := range pathaccessrules {
			if pathaccessrule == nil {
				continue
			}
			for _, method := range pathaccessrule.Methods {
				switch strings.ToUpper(strings.TrimSpace(method)) {
				case "GRPC":
					if _, ok := tmpgrpc[path]; !ok {
						tmpgrpc[path] = make([]string, 0, 3)
					}
					tmpgrpc[path] = append(tmpgrpc[path], pathaccessrule.Accesses...)
				case "CRPC":
					if _, ok := tmpcrpc[path]; !ok {
						tmpcrpc[path] = make([]string, 0, 3)
					}
					tmpcrpc[path] = append(tmpcrpc[path], pathaccessrule.Accesses...)
				case "GET":
					if _, ok := tmpget[path]; !ok {
						tmpget[path] = make([]string, 0, 3)
					}
					tmpget[path] = append(tmpget[path], pathaccessrule.Accesses...)
				case "POST":
					if _, ok := tmppost[path]; !ok {
						tmppost[path] = make([]string, 0, 3)
					}
					tmppost[path] = append(tmppost[path], pathaccessrule.Accesses...)
				case "PUT":
					if _, ok := tmpput[path]; !ok {
						tmpput[path] = make([]string, 0, 3)
					}
					tmpput[path] = append(tmpput[path], pathaccessrule.Accesses...)
				case "PATCH":
					if _, ok := tmppatch[path]; !ok {
						tmppatch[path] = make([]string, 0, 3)
					}
					tmppatch[path] = append(tmppatch[path], pathaccessrule.Accesses...)
				case "DELETE":
					if _, ok := tmpdel[path]; !ok {
						tmpdel[path] = make([]string, 0, 3)
					}
					tmpdel[path] = append(tmpdel[path], pathaccessrule.Accesses...)
				}
			}
		}
	}
	grpcAccess.Store(&tmpgrpc)
	crpcAccess.Store(&tmpcrpc)
	getAccess.Store(&tmpget)
	postAccess.Store(&tmppost)
	putAccess.Store(&tmpput)
	patchAccess.Store(&tmppatch)
	delAccess.Store(&tmpdel)
}
func VerifyAccessKey(ctx context.Context, method, path, accesskey string) bool {
	var tmp *map[string][]string
	switch strings.ToUpper(method) {
	case "GRPC":
		tmp = grpcAccess.Load()
	case "CRPC":
		tmp = crpcAccess.Load()
	case "GET":
		tmp = getAccess.Load()
	case "POST":
		tmp = postAccess.Load()
	case "PUT":
		tmp = putAccess.Load()
	case "PATCH":
		tmp = patchAccess.Load()
	case "DELETE":
		tmp = delAccess.Load()
	default:
		return false
	}
	if tmp == nil {
		return false
	}
	accesses, ok := (*tmp)[path]
	if !ok {
		return false
	}
	return slices.Contains(accesses, accesskey)
}
