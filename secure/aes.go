package secure

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/common"
)

func AesEncrypt(password string, plaintxt []byte) (string, error) {
	if len(password) >= 32 {
		return "", cerror.ErrPasswordLength
	}
	s := padding(common.Str2byte(password), 32)
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCM(block)
	nonce := make([]byte, aead.NonceSize())
	rand.Read(nonce)
	ciphertext := aead.Seal(nil, nonce, plaintxt, nil)
	return hex.EncodeToString(append(nonce, ciphertext...)), nil
}
func AesDecrypt(password string, ciphertxt string) ([]byte, error) {
	if len(password) >= 32 {
		return nil, cerror.ErrPasswordLength
	}
	tmp, e := hex.DecodeString(ciphertxt)
	if e != nil {
		return nil, cerror.ErrDataBroken
	}
	s := padding(common.Str2byte(password), 32)
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCM(block)
	plaintext, e := aead.Open(nil, tmp[:aead.NonceSize()], tmp[aead.NonceSize():], nil)
	if e != nil {
		return nil, cerror.ErrPasswordWrong
	}
	return plaintext, nil
}

func padding(origin []byte, size uint8) []byte {
	padding := int(size) - len(origin)%int(size)
	return append(origin, bytes.Repeat([]byte{byte(padding)}, padding)...)
}
func unpadding(origin []byte, size uint8) []byte {
	length := len(origin)
	unpadding := uint8(origin[length-1])
	if unpadding > size {
		return nil
	}
	return origin[:(length - int(unpadding))]
}
