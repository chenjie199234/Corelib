package common

import (
	"math/rand"
	"unsafe"
)

func STB(data string) []byte {
	return unsafe.Slice(unsafe.StringData(data), len(data))
}
func BTS(data []byte) string {
	return unsafe.String(unsafe.SliceData(data), len(data))
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789#*" //letters' length is 64 and 2^6=64

func MakeRandCode(length uint16) string {
	if length == 0 {
		return ""
	}
	b := make([]byte, length)
	for {
		r := rand.Uint64()
		for i := range 59 {
			b[length-1] = letters[((r>>i)<<58)>>58]
			length--
			if length == 0 {
				break
			}
		}
		if length == 0 {
			break
		}
	}
	return BTS(b)
}
