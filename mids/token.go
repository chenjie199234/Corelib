package mids

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/secure"
)

type Token struct {
	Puber     string `json:"p"`
	DeployEnv string `json:"d_env"` //deploy location,example: ali-xxx,aws-xxx
	RunEnv    string `json:"r_env"` //example: test,dev,prod...
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
func MakeToken(ctx context.Context, puber, deployenv, runenv, data string) string {
	start := time.Now()
	end := start.Add(tokenexpire)
	t, _ := json.Marshal(&Token{
		Puber:     puber,
		DeployEnv: deployenv,
		RunEnv:    runenv,
		Data:      data,
		Start:     uint64(start.Unix()),
		End:       uint64(end.Unix()),
	})
	ciphertext, e := secure.AesEncrypt(tokensecret, t)
	if e != nil {
		log.Error(ctx, "[token.make] token secret length less then 32", nil)
		return ""
	}
	return base64.StdEncoding.EncodeToString(ciphertext)
}
func VerifyToken(ctx context.Context, tokenstr string) *Token {
	ciphertext, e := base64.StdEncoding.DecodeString(tokenstr)
	if e != nil {
		return nil
	}
	plaintext, e := secure.AesDecrypt(tokensecret, ciphertext)
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
