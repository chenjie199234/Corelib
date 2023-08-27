package secure

import (
	"bytes"
)

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
