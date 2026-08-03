package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"
)

func AesEncrypt(password string, plaintxt []byte) (string, error) {
	salt := make([]byte, 32)
	rand.Read(salt)
	h := GetSha256()
	h.Write(salt)
	h.Write(common.STB(password))
	key := h.Sum(nil)
	PutSha256(h)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	ciphertext := aead.Seal(nil, nonce, plaintxt, nil)
	tmp := make([]byte, len(salt)+len(nonce)+len(ciphertext))
	copy(tmp, salt)
	copy(tmp[32:], nonce)
	copy(tmp[32+len(nonce):], ciphertext)
	return hex.EncodeToString(tmp), nil
}
func AesDecrypt(password string, ciphertxt string) ([]byte, error) {
	tmp, e := hex.DecodeString(ciphertxt)
	if e != nil {
		return nil, cerror.ErrDataBroken
	}
	if len(tmp) < 32 {
		return nil, cerror.ErrDataBroken
	}
	h := GetSha256()
	h.Write(tmp[:32])
	h.Write(common.STB(password))
	key := h.Sum(nil)
	PutSha256(h)
	tmp = tmp[32:]
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	if len(tmp) < aead.NonceSize()+aead.Overhead() {
		return nil, cerror.ErrDataBroken
	}
	plaintext, e := aead.Open(nil, tmp[:aead.NonceSize()], tmp[aead.NonceSize():], nil)
	if e != nil {
		return nil, cerror.ErrPasswordWrong
	}
	return plaintext, nil
}
