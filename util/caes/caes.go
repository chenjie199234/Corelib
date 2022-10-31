package caes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"math/rand"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}
func pkcs7Padding(origin []byte, size uint8) []byte {
	padding := int(size) - len(origin)%int(size)
	return append(origin, bytes.Repeat([]byte{byte(padding)}, padding)...)
}
func pkcs7UnPadding(origin []byte, size uint8) []byte {
	length := len(origin)
	unpadding := uint8(origin[length-1])
	if unpadding > size || unpadding <= 0 {
		return nil
	}
	return origin[:(length - int(unpadding))]
}

var ErrSecretTooLong = errors.New("secret too long")

// max secret len is 31
func Encrypt(secret string, plaintext []byte) ([]byte, error) {
	if len(secret) >= 32 {
		return nil, ErrSecretTooLong
	}
	s := pkcs7Padding(common.Str2byte(secret), 32)
	tmp := pkcs7Padding([]byte(plaintext), aes.BlockSize)

	ciphertext := make([]byte, aes.BlockSize+len(tmp))
	rand.Read(ciphertext[:aes.BlockSize])

	block, _ := aes.NewCipher(s)
	cipher.NewCBCEncrypter(block, ciphertext[:aes.BlockSize]).CryptBlocks(ciphertext[aes.BlockSize:], tmp)
	return ciphertext, nil
}

var ErrCipherDataBroken = errors.New("cipher text broken")

// max secret len is 31
func Decrypt(secret string, ciphertext []byte) ([]byte, error) {
	if len(secret) >= 32 {
		return nil, ErrSecretTooLong
	}
	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return nil, ErrCipherDataBroken
	}
	s := pkcs7Padding(common.Str2byte(secret), 32)
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	block, _ := aes.NewCipher(s)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(ciphertext, ciphertext)
	return pkcs7UnPadding(ciphertext, aes.BlockSize), nil
}
