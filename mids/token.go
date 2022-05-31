package mids

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
)

type Token struct {
	Puber     string `json:"puber"`
	DeployEnv string `json:"d_env"` //deploy location,example: ali-xxx,aws-xxx
	RunEnv    string `json:"r_env"` //example: test,dev,prod...
	Data      string `json:"data"`
	Start     uint64 `json:"start"` //timestamp,unit is second
	End       uint64 `json:"end"`   //timestamp,unit is second
	Sig       string `json:"sig"`
}

func MakeToken(secret, puber, deployenv, runenv, data string, start, end uint64) (tokenstr string) {
	t := &Token{
		Puber:     puber,
		DeployEnv: deployenv,
		RunEnv:    runenv,
		Data:      data,
		Start:     start,
		End:       end,
		Sig:       secret,
	}
	d, _ := json.Marshal(t)
	h := sha256.Sum256(d)
	t.Sig = hex.EncodeToString(h[:])
	d, _ = json.Marshal(t)
	return base64.RawStdEncoding.EncodeToString(d)
}

func VerifyToken(secret, tokenstr string) (*Token, error) {
	tokenbytes, e := base64.RawStdEncoding.DecodeString(tokenstr)
	if e != nil {
		return nil, cerror.ErrAuth
	}
	t := &Token{}
	if e := json.Unmarshal(tokenbytes, t); e != nil {
		return nil, cerror.ErrAuth
	}
	now := uint64(time.Now().Unix())
	if t.Start > now || t.End <= now {
		return nil, cerror.ErrAuth
	}
	sig := t.Sig
	t.Sig = secret
	d, _ := json.Marshal(t)
	h := sha256.Sum256(d)
	if hex.EncodeToString(h[:]) != sig {
		return nil, cerror.ErrAuth
	}
	t.Sig = sig
	return t, nil
}
