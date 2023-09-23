package mids

import (
	"context"
	"encoding/json"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/secure"
)

type Token struct {
	Puber     string `json:"p"`
	DeployEnv string `json:"d_env"` //deploy location,example: ali-xxx,aws-xxx
	RunEnv    string `json:"r_env"` //example: test,dev,prod...
	UserID    string `json:"u"`
	Data      string `json:"d"`
	Start     uint64 `json:"s"` //timestamp,unit is second
	End       uint64 `json:"e"` //timestamp,unit is second
}

var tokensecret string
var tokenexpire time.Duration

func UpdateTokenConfig(secret string, expire time.Duration) {
	tokensecret = secret
	tokenexpire = expire
}

// return empty means make token failed
// put the return data in web's Token header or metadata's Token field
func MakeToken(ctx context.Context, puber, deployenv, runenv, userid, data string) string {
	start := time.Now()
	end := start.Add(tokenexpire)
	t, _ := json.Marshal(&Token{
		Puber:     puber,
		DeployEnv: deployenv,
		RunEnv:    runenv,
		UserID:    userid,
		Data:      data,
		Start:     uint64(start.Unix()),
		End:       uint64(end.Unix()),
	})
	tokenstr, e := secure.AesEncrypt(tokensecret, t)
	if e != nil {
		log.Error(ctx, "[token.make] failed", log.CError(e))
		return ""
	}
	return tokenstr
}
func VerifyToken(ctx context.Context, tokenstr string) *Token {
	plaintext, e := secure.AesDecrypt(tokensecret, tokenstr)
	if e != nil {
		return nil
	}
	t := &Token{}
	if e := json.Unmarshal(plaintext, t); e != nil {
		return nil
	}
	now := uint64(time.Now().Unix())
	if t.Start > now || t.End <= now {
		return nil
	}
	return t
}
