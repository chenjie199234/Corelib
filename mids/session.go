package mids

import (
	"encoding/hex"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/redis"
	"golang.org/x/net/context"
)

type session struct {
	p      *redis.Pool
	expire int64
}

var sessioninstance *session

func init() {
	rand.Seed(time.Now().UnixNano())
	sessioninstance = &session{}
}

func UpdateSessionConfig(redisurl string, expire time.Duration) {
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
		log.Warning(nil, "[session] config missing redis url,all session event will be failed")
	}
	oldp := (*redis.Pool)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessioninstance.p)), unsafe.Pointer(newp)))
	if oldp != nil {
		oldp.Close()
	}
	if expire.Seconds() < 1 {
		log.Warning(nil, "[session] expire too small")
	}
	sessioninstance.expire = int64(expire.Seconds())
}

// return empty means make session failed
func MakeSession(ctx context.Context, userid, data string) string {
	if sessioninstance.p == nil {
		log.Error(ctx, "[session.make] config missing redis url")
		return ""
	}
	if sessioninstance.expire < 1 {
		log.Error(ctx, "[session.make] expire too small")
		return ""
	}
	result := make([]byte, 8)
	rand.Read(result)
	sessionid := hex.EncodeToString(result)
	conn, e := sessioninstance.p.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.make] get redis conn:", e)
		return ""
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "SETEX", "session_"+userid, sessioninstance.expire, sessionid+"_"+data); e != nil {
		log.Error(ctx, "[session.make] write redis session data:", e)
		return ""
	}
	return sessionid
}

func CleanSession(ctx context.Context, userid string) bool {
	if sessioninstance.p == nil {
		log.Error(ctx, "[session.clean] config missing redis url")
		return false
	}
	conn, e := sessioninstance.p.GetContext(ctx)
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

func ExtendSession(ctx context.Context, userid string) bool {
	if sessioninstance.p == nil {
		log.Error(ctx, "[session.extend] config missing redis url")
		return false
	}
	if sessioninstance.expire < 1 {
		log.Error(ctx, "[session.make] expire too small")
		return false
	}
	conn, e := sessioninstance.p.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.extend] get redis conn:", e)
		return false
	}
	defer conn.Close()
	if _, e = conn.DoContext(ctx, "EXPIRE", "session_"+userid, sessioninstance.expire); e != nil {
		log.Error(ctx, "[session.extend] update redis session data:", e)
		return false
	}
	return true
}

func VerifySession(ctx context.Context, userid, sessionid string) (bool, string) {
	if sessioninstance.p == nil {
		log.Error(ctx, "[session.verify] config missing redis url")
		return false, ""
	}
	conn, e := sessioninstance.p.GetContext(ctx)
	if e != nil {
		log.Error(ctx, "[session.verify] get redis conn:", e)
		return false, ""
	}
	defer conn.Close()
	str, e := redis.String(conn.DoContext(ctx, "GET", "session_"+userid))
	if e != nil {
		if e != redis.ErrNil {
			log.Error(ctx, "[session.verify] read redis session data:", e)
		}
		return false, ""
	}
	if !strings.HasPrefix(str, sessionid) {
		return false, ""
	}
	return true, str[17:]
}
