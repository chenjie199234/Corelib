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
		log.Warning(nil, "[session] redis missing,all session event will be failed", nil)
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessionredis)), unsafe.Pointer(newp)))
	if oldp != nil {
		oldp.Close()
	}
}
func UpdateSessionRedisInstance(p *redis.Pool) {
	if p == nil {
		log.Warning(nil, "[session] redis missing,all session event will be failed", nil)
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessionredis)), unsafe.Pointer(p)))
	if oldp != nil {
		oldp.Close()
	}
}

// return empty means make session failed
// user should put the return data in web's Session header or metadata's Session field
func MakeSession(ctx context.Context, userid, data string) string {
	redisclient := sessionredis
	if redisclient == nil {
		log.Error(ctx, "[session.make] redis missing", nil)
		return ""
	}
	result := make([]byte, 8)
	rand.Read(result)
	sessionid := hex.EncodeToString(result)
	conn, e := redisclient.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.make] get redis conn failed", map[string]interface{}{"error": e})
		return ""
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "PSETEX", "session_"+userid, sessionexpire.Milliseconds(), sessionid+"_"+data); e != nil {
		log.Error(ctx, "[session.make] write session data failed", map[string]interface{}{"error": e})
		return ""
	}
	return "userid=" + userid + ",sessionid=" + sessionid
}

func CleanSession(ctx context.Context, userid string) bool {
	redisclient := sessionredis
	if redisclient == nil {
		log.Error(ctx, "[session.clean] redis missing", nil)
		return false
	}
	conn, e := redisclient.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.clean] get redis conn failed", map[string]interface{}{"error": e})
		return false
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "DEL", "session_"+userid); e != nil {
		log.Error(ctx, "[session.clean] delete session data failed", map[string]interface{}{"error": e})
		return false
	}
	return true
}

func ExtendSession(ctx context.Context, userid string, expire time.Duration) bool {
	redisclient := sessionredis
	if redisclient == nil {
		log.Error(ctx, "[session.extend] redis missing", nil)
		return false
	}
	conn, e := redisclient.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.extend] get redis conn failed", map[string]interface{}{"error": e})
		return false
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "PEXPIRE", "session_"+userid, expire.Milliseconds()); e != nil {
		log.Error(ctx, "[session.extend] update session data failed", map[string]interface{}{"error": e})
		return false
	}
	return true
}

func VerifySession(ctx context.Context, sessionstr string) (string, string, bool) {
	redisclient := sessionredis
	if redisclient == nil {
		log.Error(ctx, "[session.verify] redis missing", nil)
		return "", "", false
	}
	index := strings.LastIndex(sessionstr, ",")
	if index == -1 {
		return "", "", false
	}
	userid := sessionstr[:index]
	sessionid := sessionstr[index+1:]
	if !strings.HasPrefix(userid, "userid=") {
		return "", "", false
	}
	if !strings.HasPrefix(sessionid, "sessionid=") {
		return "", "", false
	}
	userid = userid[7:]
	sessionid = sessionid[10:]
	conn, e := redisclient.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.verify] get redis conn failed", map[string]interface{}{"error": e})
		return "", "", false
	}
	defer conn.Close()
	str, e := redis.String(conn.DoContext(ctx, "GET", "session_"+userid))
	if e != nil {
		if e != redis.ErrNil {
			log.Error(ctx, "[session.verify] read session data failed", map[string]interface{}{"error": e})
		}
		return "", "", false
	}
	if !strings.HasPrefix(str, sessionid+"_") {
		return "", "", false
	}
	if len(str) < 17 {
		return "", "", false
	}
	return userid, str[17:], true
}
