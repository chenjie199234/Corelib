package mids

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/redis"
)

var sessionredis *redis.Client

func UpdateSessionRedisInstance(c *redis.Client) {
	if c == nil {
		slog.WarnContext(nil, "[session] redis missing,all session event will be failed")
	}
	oldp := (*redis.Client)(atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&sessionredis)), unsafe.Pointer(c)))
	if oldp != nil {
		oldp.Close()
	}
}

// return empty means make session failed
// user should put the return data in web's Session header or metadata's Session field
func MakeSession(ctx context.Context, userid, data string,expire time.Duration) string {
	redisclient := sessionredis
	if redisclient == nil {
		slog.ErrorContext(ctx, "[session.make] redis missing")
		return ""
	}
	result := make([]byte, 8)
	rand.Read(result)
	sessionid := hex.EncodeToString(result)
	if _, e := redisclient.SetEx(ctx, "session_"+userid, sessionid+"_"+data, expire).Result(); e != nil {
		slog.ErrorContext(ctx, "[session.make] write session data failed", slog.String("userid", userid), slog.String("error", e.Error()))
		return ""
	}
	return "userid=" + userid + ",sessionid=" + sessionid
}

func CleanSession(ctx context.Context, userid string) bool {
	redisclient := sessionredis
	if redisclient == nil {
		slog.ErrorContext(ctx, "[session.clean] redis missing")
		return false
	}
	if _, e := redisclient.Del(ctx, "session_"+userid).Result(); e != nil {
		slog.ErrorContext(ctx, "[session.clean] delete session data failed", slog.String("userid", userid), slog.String("error", e.Error()))
		return false
	}
	return true
}

func ExtendSession(ctx context.Context, userid string, expire time.Duration) bool {
	redisclient := sessionredis
	if redisclient == nil {
		slog.ErrorContext(ctx, "[session.extend] redis missing")
		return false
	}
	if _, e := redisclient.Expire(ctx, "session_"+userid, expire).Result(); e != nil {
		slog.ErrorContext(ctx, "[session.extend] update session data failed", slog.String("userid", userid), slog.String("error", e.Error()))
		return false
	}
	return true
}

func VerifySession(ctx context.Context, sessionstr string) (string, string, bool) {
	redisclient := sessionredis
	if redisclient == nil {
		slog.ErrorContext(ctx, "[session.verify] redis missing")
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
	str, e := redisclient.Get(ctx, "session_"+userid).Result()
	if e != nil {
		slog.ErrorContext(ctx, "[session.verify] read session data failed", slog.String("userid", userid), slog.String("error", e.Error()))
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
