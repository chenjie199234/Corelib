package mids

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
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
		log.Warn(nil, "[access.sign] redis missing,replay attack may happened")
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
		log.Error(ctx, "[access.key] missing init,please use UpdateAccessConfig first")
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

// put the return data in web's Sign header or metadata's Sign field
func MakeAccessSign(accessid, accesskey, method, path string, querys url.Values, headers http.Header, metadata map[string]string, body []byte) string {
	nonce := rand.Int63()
	timestamp := time.Now().Unix()
	buf := pool.GetPool().Get(len(accesskey) + len(path) + 51)
	defer pool.GetPool().Put(&buf)
	buf = buf[:0]
	buf = append(buf, accesskey...)
	buf = append(buf, '\n')
	buf = strconv.AppendInt(buf, nonce, 10)
	buf = append(buf, '\n')
	buf = strconv.AppendInt(buf, timestamp, 10)
	buf = append(buf, '\n')
	buf = append(buf, method...)
	buf = append(buf, '\n')
	if path == "" {
		buf = append(buf, '/')
	} else {
		if path[0] != '/' {
			buf = append(buf, '/')
		}
		buf = append(buf, path...)
		if path[len(path)-1] != '/' {
			buf = append(buf, '/')
		}
	}
	buf = append(buf, '\n')

	//query
	querykeys := make([]string, 0, len(querys))
	tmpquerys := make(map[string][]string)
	for k, vs := range querys {
		querykeys = append(querykeys, k)
		sort.Strings(vs)
		tmpquerys[k] = vs
	}
	sort.Strings(querykeys)
	for i, querykey := range querykeys {
		vs := tmpquerys[querykey]
		for j, v := range vs {
			ek := url.QueryEscape(querykey)
			ev := url.QueryEscape(v)
			if i != 0 || j != 0 {
				buf = pool.CheckCap(&buf, len(buf)+len(ek)+len(ek)+2)
				buf = append(buf, '&')
			} else {
				buf = pool.CheckCap(&buf, len(buf)+len(ek)+len(ek)+1)
			}
			buf = append(buf, ek...)
			buf = append(buf, '=')
			buf = append(buf, ev...)
		}
	}
	buf = pool.CheckCap(&buf, len(buf)+1)
	buf = append(buf, '\n')

	//header
	headerkeys := make([]string, 0, len(headers))
	tmpheaders := make(map[string][]string)
	for k, vs := range headers {
		k = strings.TrimSpace(strings.ToLower(k))
		headerkeys = append(headerkeys, k)
		for i := range vs {
			vs[i] = strings.TrimSpace(vs[i])
		}
		sort.Strings(vs)
		tmpheaders[k] = vs
	}
	sort.Strings(headerkeys)
	for _, headerkey := range headerkeys {
		vs := tmpheaders[headerkey]
		for _, v := range vs {
			buf = pool.CheckCap(&buf, len(buf)+len(headerkey)+len(v)+2)
			buf = append(buf, headerkey...)
			buf = append(buf, ':')
			buf = append(buf, v...)
			buf = append(buf, '\n')
		}
	}

	//metadata
	mdkeys := make([]string, 0, len(metadata))
	for k := range metadata {
		mdkeys = append(mdkeys, k)
	}
	sort.Strings(mdkeys)
	for _, mdkey := range mdkeys {
		v := metadata[mdkey]
		buf = pool.CheckCap(&buf, len(buf)+len(mdkey)+len(v)+2)
		buf = append(buf, mdkey...)
		buf = append(buf, ':')
		buf = append(buf, v...)
		buf = append(buf, '\n')
	}

	//body
	bodydigest := sha256.Sum256(body)
	buf = pool.CheckCap(&buf, len(buf)+base64.StdEncoding.EncodedLen(sha256.Size))
	curlen := len(buf)
	buf = buf[:curlen+base64.StdEncoding.EncodedLen(sha256.Size)]
	base64.StdEncoding.Encode(buf[curlen:], bodydigest[:])

	//all request
	reqdigest := sha256.Sum256(buf)
	sign := base64.StdEncoding.EncodeToString(reqdigest[:])

	buf = buf[:0]
	buf = append(buf, "A="...)
	buf = append(buf, accessid...)
	buf = append(buf, ",N="...)
	buf = strconv.AppendInt(buf, nonce, 10)
	buf = append(buf, ",T="...)
	buf = strconv.AppendInt(buf, timestamp, 10)
	buf = append(buf, ",H="...)
	for i, headerkey := range headerkeys {
		if i != 0 {
			buf = append(buf, ';')
		}
		buf = append(buf, headerkey...)
	}
	buf = append(buf, ",M="...)
	for i, mdkey := range mdkeys {
		if i != 0 {
			buf = append(buf, ';')
		}
		buf = append(buf, mdkey...)
	}
	buf = append(buf, ",S="...)
	buf = append(buf, sign...)
	signstr := string(buf)
	return signstr
}
func getAccesses(ctx context.Context, method, path, accessid string) string {
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
		return ""
	}
	if tmp == nil {
		log.Error(ctx, "[access.sign] missing init,please use UpdateAccessConfig first")
		return ""
	}
	accesses, ok := tmp[path]
	if !ok {
		return ""
	}
	return accesses[accessid]
}
func VerifyAccessSign(ctx context.Context, method, path string, querys url.Values, headers http.Header, metadata map[string]string, body []byte, signstr string, clientip string) bool {
	accessid, nonce, timestamp, headerkeys, mdkeys, sign := parseSignstr(signstr)
	now := time.Now().Unix()
	if accessid == "" || nonce == "" || timestamp+5 < now || sign == "" {
		return false
	}
	accesskey := getAccesses(ctx, method, path, accessid)
	if accesskey == "" {
		return false
	}
	buf := pool.GetPool().Get(len(accesskey) + len(nonce) + len(path) + 32)
	defer pool.GetPool().Put(&buf)
	buf = buf[:0]
	buf = append(buf, accesskey...)
	buf = append(buf, '\n')
	buf = append(buf, nonce...)
	buf = append(buf, '\n')
	buf = strconv.AppendInt(buf, timestamp, 10)
	buf = append(buf, '\n')
	buf = append(buf, method...)
	buf = append(buf, '\n')
	if path == "" {
		buf = append(buf, '/')
	} else {
		if path[0] != '/' {
			buf = append(buf, '/')
		}
		buf = append(buf, path...)
		if path[len(path)-1] != '/' {
			buf = append(buf, '/')
		}
	}
	buf = append(buf, '\n')

	//query
	querykeys := make([]string, 0, len(querys))
	tmpquerys := make(map[string][]string)
	for k, vs := range querys {
		querykeys = append(querykeys, k)
		sort.Strings(vs)
		tmpquerys[k] = vs
	}
	sort.Strings(querykeys)
	for i, querykey := range querykeys {
		vs := tmpquerys[querykey]
		for j, v := range vs {
			ek := url.QueryEscape(querykey)
			ev := url.QueryEscape(v)
			if i != 0 || j != 0 {
				buf = pool.CheckCap(&buf, len(buf)+len(ek)+len(ev)+2)
				buf = append(buf, '&')
			} else {
				buf = pool.CheckCap(&buf, len(buf)+len(ek)+len(ev)+1)
			}
			buf = append(buf, ek...)
			buf = append(buf, '=')
			buf = append(buf, ev...)
		}
	}
	buf = pool.CheckCap(&buf, len(buf)+1)
	buf = append(buf, '\n')

	//header
	for _, headerkey := range headerkeys {
		vs := headers.Values(headerkey)
		for _, v := range vs {
			buf = pool.CheckCap(&buf, len(buf)+len(headerkey)+len(v)+2)
			buf = append(buf, headerkey...)
			buf = append(buf, ':')
			buf = append(buf, v...)
			buf = append(buf, '\n')
		}
	}

	//metadata
	for _, mdkey := range mdkeys {
		v := metadata[mdkey]
		buf = pool.CheckCap(&buf, len(buf)+len(mdkey)+len(v)+2)
		buf = append(buf, mdkey...)
		buf = append(buf, ':')
		buf = append(buf, v...)
		buf = append(buf, '\n')
	}

	//body
	bodydigest := sha256.Sum256(body)
	buf = pool.CheckCap(&buf, len(buf)+base64.StdEncoding.EncodedLen(sha256.Size))
	curlen := len(buf)
	buf = buf[:curlen+base64.StdEncoding.EncodedLen(sha256.Size)]
	base64.StdEncoding.Encode(buf[curlen:], bodydigest[:])

	//all request
	reqdigest := sha256.Sum256(buf)
	if base64.StdEncoding.EncodeToString(reqdigest[:]) != sign {
		return false
	}
	redisclient := accessInstance.c
	if redisclient == nil {
		//if there is no replay defend redis
		//jump this check
		return true
	}
	//check replay attack
	status, e := redisclient.SetNX(ctx, nonce, 1, time.Second*5).Result()
	if e != nil {
		log.Error(ctx, "[access.sign] replay attack check failed", log.CError(e))
		return false
	}
	if !status {
		log.Error(ctx, "[access.sign] replay attack", log.String("cip", clientip))
	}
	return status
}
func parseSignstr(signstr string) (accessid string, nonce string, timestamp int64, headers []string, mdkeys []string, sign string) {
	if !strings.HasPrefix(signstr, "A=") {
		return
	}
	index := strings.Index(signstr, ",")
	accessid = strings.TrimSpace(signstr[:index][2:])
	signstr = strings.TrimSpace(signstr[index+1:])
	if !strings.HasPrefix(signstr, "N=") {
		return
	}
	index = strings.Index(signstr, ",")
	nonce = strings.TrimSpace(signstr[:index][2:])
	signstr = strings.TrimSpace(signstr[index+1:])
	if !strings.HasPrefix(signstr, "T=") {
		return
	}
	index = strings.Index(signstr, ",")
	var e error
	timestamp, e = strconv.ParseInt(strings.TrimSpace(signstr[:index][2:]), 10, 64)
	if e != nil {
		return
	}
	signstr = strings.TrimSpace(signstr[index+1:])
	if !strings.HasPrefix(signstr, "H=") {
		return
	}
	index = strings.Index(signstr, ",")
	headerkeystr := strings.TrimSpace(signstr[:index][2:])
	if headerkeystr != "" {
		headers = strings.Split(headerkeystr, ";")
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	signstr = strings.TrimSpace(signstr[index+1:])
	if !strings.HasPrefix(signstr, "M=") {
		return
	}
	index = strings.Index(signstr, ",")
	mdkeystr := strings.TrimSpace(signstr[:index][2:])
	if mdkeystr != "" {
		mdkeys = strings.Split(mdkeystr, ";")
	}
	signstr = strings.TrimSpace(signstr[index+1:])
	if !strings.HasPrefix(signstr, "S=") {
		return
	}
	sign = strings.TrimSpace(signstr[2:])
	return
}
