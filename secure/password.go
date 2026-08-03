package secure

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"
)

func SignPassword(password string) (string, error) {
	cache := make([]byte, 64)
	//this is the salt for password
	rand.Read(cache[:32])
	h := GetSha256()
	h.Write(cache[:32])
	h.Write(common.STB(password))
	sign := h.Sum(nil)
	PutSha256(h)
	copy(cache[32:], sign)
	return hex.EncodeToString(cache), nil
}
func CheckPasswordSign(password, sign string) error {
	tmp, e := hex.DecodeString(sign)
	if e != nil {
		return cerror.ErrDataBroken
	}
	if len(tmp) != 64 {
		return cerror.ErrDataBroken
	}
	h := GetSha256()
	//salt
	h.Write(tmp[:32])
	h.Write(common.STB(password))
	newsign := h.Sum(nil)
	PutSha256(h)
	if !bytes.Equal(tmp[32:], newsign) {
		return cerror.ErrPasswordWrong
	}
	return nil
}
