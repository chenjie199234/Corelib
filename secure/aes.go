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
	if len(password) > 32 {
		return "", cerror.ErrPasswordLength
	}
	var s []byte
	if len(password) < 32 {
		s = Padding(common.STB(password), 32)
	} else {
		s = common.STB(password)
	}
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCM(block)
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	ciphertext := aead.Seal(nil, nonce, plaintxt, nil)
	return hex.EncodeToString(append(nonce, ciphertext...)), nil
}
func AesDecrypt(password string, ciphertxt string) ([]byte, error) {
	if len(password) > 32 {
		return nil, cerror.ErrPasswordLength
	}
	tmp, e := hex.DecodeString(ciphertxt)
	if e != nil {
		return nil, cerror.ErrDataBroken
	}
	var s []byte
	if len(password) < 32 {
		s = Padding(common.STB(password), 32)
	} else {
		s = common.STB(password)
	}
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCM(block)
	plaintext, e := aead.Open(nil, tmp[:aead.NonceSize()], tmp[aead.NonceSize():], nil)
	if e != nil {
		return nil, cerror.ErrPasswordWrong
	}
	return plaintext, nil
}
