package common

import (
	"math/rand"
	"unsafe"
)

func Str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func Byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789#*" //letters' length is 64 and 2^6=64

func MakeRandCode(length uint16) string {
	if length == 0 {
		return ""
	}
	b := make([]byte, length)
	for {
		r := rand.Uint64()
		for i := 0; i < 59; i++ {
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
	return Byte2str(b)
}
