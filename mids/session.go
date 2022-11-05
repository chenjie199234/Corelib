package mids

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/redis"
)

var sessionredis *redis.Pool
var sessionexpire time.Duration

func UpdateSessionConfig(expire time.Duration) {
	sessionexpire = expire
}
func UpdateSessionRedisUrl(redisurl string) {
	var newp *redis.Pool
	if redisurl != "" {
		newp = redis.NewRedis(&redis.Config{
			RedisName:   "session_redis",
			URL:         redisurl,
			MaxOpen:     0,    //means no limit
			MaxIdle:     1024, //the pool's buf
			MaxIdletime: time.Minute,
			ConnTimeout: time.Second,
			IOTimeout:   time.Second,
		})
	} else {
		log.Warning(nil, "[session] config missing redis,all session event will be failed")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessionredis)), unsafe.Pointer(newp)))
	if oldp != nil {
		oldp.Close()
	}
}
func UpdateSessionRedisInstance(p *redis.Pool) {
	if p == nil {
		log.Warning(nil, "[session] config missing redis,all session event will be failed")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessionredis)), unsafe.Pointer(p)))
	if oldp != nil {
		oldp.Close()
	}
}

// return empty means make session failed
// user should put the return data in web's Session header or metadata's Session field
func MakeSession(ctx context.Context, userid, data string) string {
	if sessionredis == nil {
		log.Error(ctx, "[session.make] config missing redis")
		return ""
	}
	result := make([]byte, 8)
	rand.Read(result)
	sessionid := hex.EncodeToString(result)
	conn, e := sessionredis.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.make] get redis conn:", e)
		return ""
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "PSETEX", "session_"+userid, sessionexpire.Milliseconds(), sessionid+"_"+data); e != nil {
		log.Error(ctx, "[session.make] write redis session data:", e)
		return ""
	}
	return "userid=" + userid + ",sessionid=" + sessionid
}

func CleanSession(ctx context.Context, userid string) bool {
	if sessionredis == nil {
		log.Error(ctx, "[session.clean] config missing redis")
		return false
	}
	conn, e := sessionredis.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.clean] get redis conn:", e)
		return false
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "DEL", "session_"+userid); e != nil {
		log.Error(ctx, "[session.clean] delete redis session data:", e)
		return false
	}
	return true
}

func ExtendSession(ctx context.Context, userid string, expire time.Duration) bool {
	if sessionredis == nil {
		log.Error(ctx, "[session.extend] config missing redis")
		return false
	}
	conn, e := sessionredis.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.extend] get redis conn:", e)
		return false
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "PEXPIRE", "session_"+userid, expire.Milliseconds()); e != nil {
		log.Error(ctx, "[session.extend] update redis session data:", e)
		return false
	}
	return true
}

func VerifySession(ctx context.Context, sessionstr string) (string, bool) {
	if sessionredis == nil {
		log.Error(ctx, "[session.verify] config missing redis")
		return "", false
	}
	index := strings.LastIndex(sessionstr, ",")
	if index == -1 {
		return "", false
	}
	userid := sessionstr[:index]
	sessionid := sessionstr[index+1:]
	if !strings.HasPrefix(userid, "userid=") {
		return "", false
	}
	if !strings.HasPrefix(sessionid, "sessionid=") {
		return "", false
	}
	userid = userid[7:]
	sessionid = sessionid[10:]
	conn, e := sessionredis.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.verify] get redis conn:", e)
		return "", false
	}
	defer conn.Close()
	str, e := redis.String(conn.DoContext(ctx, "GET", "session_"+userid))
	if e != nil {
		if e != redis.ErrNil {
			log.Error(ctx, "[session.verify] read redis session data:", e)
		}
		return "", false
	}
	if !strings.HasPrefix(str, sessionid) {
		return "", false
	}
	if len(str) < 17 {
		return "", false
	}
	return str[17:], true
}