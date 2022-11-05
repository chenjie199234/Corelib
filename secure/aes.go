package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"github.com/chenjie199234/Corelib/util/common"
)

var ErrAesSecretLength = errors.New("secret length must less then 32")
var ErrAesCipherTextBroken = errors.New("cipher text broken")

const _NONCE_SIZE = 16

// max secret length 31
func AesDecrypt(secret string, ciphertext []byte) ([]byte, error) {
	if len(secret) >= 32 {
		return nil, ErrAesSecretLength
	}
	if len(ciphertext) <= _NONCE_SIZE || (len(ciphertext)-_NONCE_SIZE)%aes.BlockSize != 0 {
		return nil, ErrAesCipherTextBroken
	}
	s := pkcs7Padding(common.Str2byte(secret), 32)
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCMWithNonceSize(block, _NONCE_SIZE)
	plaintext, e := aead.Open(nil, ciphertext[:_NONCE_SIZE], ciphertext[_NONCE_SIZE:], nil)
	if e != nil {
		return nil, e
	}
	return pkcs7UnPadding(plaintext, aes.BlockSize), nil
}

// max secret length 31
func AesEncrypt(secret string, plaintext []byte) ([]byte, error) {
	if len(secret) >= 32 {
		return nil, ErrAesSecretLength
	}
	s := pkcs7Padding(common.Str2byte(secret), 32)
	tmp := pkcs7Padding(plaintext, uint8(aes.BlockSize))
	block, _ := aes.NewCipher(s)
	aead, _ := cipher.NewGCMWithNonceSize(block, _NONCE_SIZE)
	nonce := make([]byte, _NONCE_SIZE)
	rand.Read(nonce)
	ciphertext := aead.Seal(nil, nonce, tmp, nil)
	return append(nonce, ciphertext...), nil
}
