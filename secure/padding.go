package secure

import (
	"bytes"
)

// PKCS#7
func Padding(origin []byte, size uint8) []byte {
	padding := int(size) - len(origin)%int(size)
	return append(origin, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// PKCS#7
func Unpadding(origin []byte, size uint8) []byte {
	length := len(origin)
	unpadding := uint8(origin[length-1])
	if unpadding > size {
		return nil
	}
	return origin[:(length - int(unpadding))]
}
