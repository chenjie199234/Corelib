package mids

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/chenjie199234/Corelib/log"
)

type token struct {
	secret string
	expire time.Duration
}
type Token struct {
	Puber     string `json:"puber"`
	DeployEnv string `json:"d_env"` //deploy location,example: ali-xxx,aws-xxx
	RunEnv    string `json:"r_env"` //example: test,dev,prod...
	Data      string `json:"data"`
	Start     uint64 `json:"start"` //timestamp,unit is second
	End       uint64 `json:"end"`   //timestamp,unit is second
	Sig       string `json:"sig"`
}

var tokeninstance *token

func init() {
	tokeninstance = &token{}
}
func UpdateTokenConfig(secret string, expire time.Duration) {
	if expire < 0 {
		log.Error(nil, "[token] expire can't be negitive number")
		return
	}
	if secret == "" {
		log.Error(nil, "[token] secret can't be empty")
		return
	}
	tokeninstance.secret = secret
	tokeninstance.expire = expire
}

// return empty means make token failed
func MakeToken(ctx context.Context, puber, deployenv, runenv, data string) string {
	if tokeninstance.secret == "" || tokeninstance.expire == 0 {
		log.Error(ctx, "[token.make] missing init,please use UpdateTokenConfig first")
		return ""
	}
	start := time.Now().UnixNano()
	end := start + int64(tokeninstance.expire)
	t := &Token{
		Puber:     puber,
		DeployEnv: deployenv,
		RunEnv:    runenv,
		Data:      data,
		Start:     uint64(start),
		End:       uint64(end),
		Sig:       tokeninstance.secret,
	}
	d, _ := json.Marshal(t)
	h := sha256.Sum256(d)
	t.Sig = hex.EncodeToString(h[:])
	d, _ = json.Marshal(t)
	return base64.RawStdEncoding.EncodeToString(d)
}
func VerifyToken(ctx context.Context, tokenstr string) *Token {
	if tokeninstance.secret == "" || tokeninstance.expire == 0 {
		log.Error(ctx, "[token.verify] missing init,please use UpdateTokenConfig first")
		return nil
	}
	tokenbytes, e := base64.RawStdEncoding.DecodeString(tokenstr)
	if e != nil {
		return nil
	}
	t := &Token{}
	if e := json.Unmarshal(tokenbytes, t); e != nil {
		return nil
	}
	now := uint64(time.Now().UnixNano())
	if t.Start > now || t.End <= now {
		return nil
	}
	sig := t.Sig
	t.Sig = tokeninstance.secret
	d, _ := json.Marshal(t)
	h := sha256.Sum256(d)
	if hex.EncodeToString(h[:]) != sig {
		return nil
	}
	t.Sig = sig
	return t
}
