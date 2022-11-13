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
	p     *redis.Pool
	grpc  map[string]map[string]string //first key path,second key accessid,value accesskey
	crpc  map[string]map[string]string
	get   map[string]map[string]string
	post  map[string]map[string]string
	put   map[string]map[string]string
	patch map[string]map[string]string
	del   map[string]map[string]string
}
type PathAccessConfig struct {
	Method   []string          `json:"method"`   //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	Accesses map[string]string `json:"accesses"` //key accessid,value accesskey,all method above share these accesses
}

var accessInstance *access

func init() {
	rand.Seed(time.Now().UnixNano())
	accessInstance = &access{}
}

// key path
func UpdateAccessConfig(c map[string][]*PathAccessConfig) {
	grpc := make(map[string]map[string]string)
	crpc := make(map[string]map[string]string)
	get := make(map[string]map[string]string)
	post := make(map[string]map[string]string)
	put := make(map[string]map[string]string)
	patch := make(map[string]map[string]string)
	del := make(map[string]map[string]string)
	for path, cc := range c {
		for _, ccc := range cc {
			for _, method := range ccc.Method {
				switch strings.ToUpper(method) {
				case "GRPC":
					if _, ok := grpc[path]; !ok {
						grpc[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						grpc[path][accessid] = accesskey
					}
				case "CRPC":
					if _, ok := crpc[path]; !ok {
						crpc[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						crpc[path][accessid] = accesskey
					}
				case "GET":
					if _, ok := get[path]; !ok {
						get[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						get[path][accessid] = accesskey
					}
				case "POST":
					if _, ok := post[path]; !ok {
						post[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						post[path][accessid] = accesskey
					}
				case "PUT":
					if _, ok := put[path]; !ok {
						put[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						put[path][accessid] = accesskey
					}
				case "PATCH":
					if _, ok := patch[path]; !ok {
						patch[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
						patch[path][accessid] = accesskey
					}
				case "DELETE":
					if _, ok := del[path]; !ok {
						del[path] = make(map[string]string)
					}
					for accessid, accesskey := range ccc.Accesses {
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
func UpdateReplayDefendRedisUrl(redisurl string) {
	var newp *redis.Pool
	if redisurl != "" {
		newp = redis.NewRedis(&redis.Config{
			RedisName:   "replay_defend_redis",
			URL:         redisurl,
			MaxOpen:     0,    //means no limit
			MaxIdle:     1024, //the pool's buf
			MaxIdletime: time.Minute,
			ConnTimeout: time.Second,
			IOTimeout:   time.Second,
		})
	} else {
		log.Warning(nil, "[access.sign] redis missing,replay attack may happened")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&accessInstance.p)), unsafe.Pointer(newp)))
	if oldp != nil {
		oldp.Close()
	}
}
func UpdateReplayDefendRedisInstance(p *redis.Pool) {
	if p == nil {
		log.Warning(nil, "[access.sign] redis missing,replay attack may happened")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&accessInstance.p)), unsafe.Pointer(p)))
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

// put the return data in web's AccessSign header or metadata's AccessSign field
func MakeAccessSign(accessid, accesskey, method, path string, querys url.Values, headers http.Header, metadata map[string]string, body []byte) string {
	nonce := rand.Int63()
	timestamp := time.Now().Unix()
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	buf.AppendString(accesskey)
	buf.AppendByte('\n')
	buf.AppendInt64(nonce)
	buf.AppendByte('\n')
	buf.AppendInt64(timestamp)
	buf.AppendByte('\n')
	buf.AppendString(method)
	buf.AppendByte('\n')
	if path == "" {
		buf.AppendByte('/')
	} else {
		if path[0] != '/' {
			buf.AppendByte('/')
		}
		buf.AppendString(path)
		if path[len(path)-1] != '/' {
			buf.AppendByte('/')
		}
	}
	buf.AppendByte('\n')
	buf.AppendString(querys.Encode())
	buf.AppendByte('\n')
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
			buf.AppendString(headerkey)
			buf.AppendByte(':')
			buf.AppendString(v)
			buf.AppendByte('\n')
		}
	}
	mdkeys := make([]string, 0, len(metadata))
	for k := range metadata {
		mdkeys = append(mdkeys, k)
	}
	sort.Strings(mdkeys)
	for _, mdkey := range mdkeys {
		buf.AppendString(mdkey)
		buf.AppendByte(':')
		buf.AppendString(metadata[mdkey])
		buf.AppendByte('\n')
	}
	buf.AppendByte('\n')
	bodydigest := sha256.Sum256(body)
	buf.AppendString(base64.StdEncoding.EncodeToString(bodydigest[:]))
	reqdigest := sha256.Sum256(buf.Bytes())
	sign := base64.StdEncoding.EncodeToString(reqdigest[:])
	buf.Reset()
	buf.AppendString("A=")
	buf.AppendString(accessid)
	buf.AppendByte(',')
	buf.AppendString("N=")
	buf.AppendInt64(nonce)
	buf.AppendByte(',')
	buf.AppendString("T=")
	buf.AppendInt64(timestamp)
	buf.AppendByte(',')
	buf.AppendString("H=")
	for i, headerkey := range headerkeys {
		buf.AppendString(headerkey)
		if i != len(headerkey)-1 {
			buf.AppendByte(';')
		}
	}
	buf.AppendByte(',')
	buf.AppendString("M=")
	for i, mdkey := range mdkeys {
		buf.AppendString(mdkey)
		if i != len(mdkeys)-1 {
			buf.AppendByte(';')
		}
	}
	buf.AppendByte(',')
	buf.AppendString("S=")
	buf.AppendString(sign)
	signstr := buf.CopyString()
	pool.PutBuffer(buf)
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
func VerifyAccessSign(ctx context.Context, method, path string, querys url.Values, headers http.Header, metadata map[string]string, body []byte, signstr string) bool {
	accessid, nonce, timestamp, headerkeys, mdkeys, sign := parseSignstr(signstr)
	now := time.Now().Unix()
	if accessid == "" || nonce == "" || timestamp+5 < now || sign == "" {
		return false
	}
	accesskey := getAccesses(ctx, method, path, accessid)
	if accesskey == "" {
		return false
	}
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	buf.AppendString(accesskey)
	buf.AppendByte('\n')
	buf.AppendString(nonce)
	buf.AppendByte('\n')
	buf.AppendInt64(timestamp)
	buf.AppendByte('\n')
	buf.AppendString(method)
	buf.AppendByte('\n')
	if path == "" {
		buf.AppendByte('/')
	} else {
		if path[0] != '/' {
			buf.AppendByte('/')
		}
		buf.AppendString(path)
		if path[len(path)-1] != '/' {
			buf.AppendByte('/')
		}
	}
	buf.AppendByte('\n')
	buf.AppendString(querys.Encode())
	buf.AppendByte('\n')
	for _, headerkey := range headerkeys {
		vs := headers.Values(headerkey)
		for _, v := range vs {
			buf.AppendString(headerkey)
			buf.AppendByte(':')
			buf.AppendString(v)
			buf.AppendByte('\n')
		}
	}
	for _, mdkey := range mdkeys {
		buf.AppendString(mdkey)
		buf.AppendByte(':')
		buf.AppendString(metadata[mdkey])
		buf.AppendByte('\n')
	}
	buf.AppendByte('\n')
	bodydigest := sha256.Sum256(body)
	buf.AppendString(base64.StdEncoding.EncodeToString(bodydigest[:]))
	reqdigest := sha256.Sum256(buf.Bytes())
	if base64.StdEncoding.EncodeToString(reqdigest[:]) != sign {
		return false
	}
	redisclient := accessInstance.p
	if redisclient == nil {
		//if there is no replay defend redis
		//jump this check
		return true
	}
	//check replay attack
	conn, e := redisclient.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[access.sign] get redis conn:", e)
		return false
	}
	defer conn.Close()
	if _, e := redis.String(conn.DoContext(ctx, "SET", nonce, 1, "EX", 5, "NX")); e != nil {
		if e != redis.ErrNil {
			log.Error(ctx, "[access.sign] write redis:", e)
		} else {
			log.Error(ctx, "[access.sign] nonce:", nonce, "duplicate")
		}
		return false
	}
	return true
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
