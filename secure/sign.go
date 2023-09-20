package secure

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"

	"github.com/chenjie199234/Corelib/cerror"
)

func SignMake(password string) (string, error) {
	if len(password) >= 32 {
		return "", cerror.ErrPasswordLength
	}
	cache := make([]byte, 64+64)
	rand.Read(cache[:64])
	copy(cache[64:], password)
	sign := sha512.Sum512(cache[:64+len(password)])
	copy(cache[64:], sign[:])
	return hex.EncodeToString(cache), nil
}
func SignCheck(password, sign string) error {
	if len(password) >= 32 {
		return cerror.ErrPasswordLength
	}
	noncesign, e := hex.DecodeString(sign)
	if e != nil {
		return cerror.ErrDataBroken
	}
	if len(noncesign) != 128 {
		return cerror.ErrDataBroken
	}
	oldsign := make([]byte, 64)
	copy(oldsign, noncesign[64:])
	copy(noncesign[64:], password)
	newsign := sha512.Sum512(noncesign[:64+len(password)])
	if !bytes.Equal(oldsign, newsign[:]) {
		return cerror.ErrPasswordWrong
	}
	return nil
}
