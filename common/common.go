package common

import (
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
func BkdrhashString(data string, total uint64) uint64 {
	seed := uint64(131)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
func DjbhashString(data string, total uint64) uint64 {
	hash := uint64(5381)
	for _, v := range data {
		hash = (hash << 5) + uint64(v)
	}
	return hash % total
}
func FnvhashString(data string, total uint64) uint64 {
	hash := uint64(2166136261)
	for _, v := range data {
		hash *= uint64(16777619)
		hash ^= uint64(v)
	}
	return hash % total
}
func DekhashString(data string, total uint64) uint64 {
	hash := uint64(len(data))
	for _, v := range data {
		hash = ((hash << 5) ^ (hash >> 27)) ^ uint64(v)
	}
	return hash % total
}
func RshashString(data string, total uint64) uint64 {
	seed := uint64(63689)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
		seed *= uint64(378551)
	}
	return hash % total
}
func SdbmhashString(data string, total uint64) uint64 {
	hash := uint64(0)
	for _, v := range data {
		hash = uint64(v) + (hash << 6) + (hash << 16) - hash
	}
	return hash % total
}
func BkdrhashByte(data []byte, total uint64) uint64 {
	seed := uint64(131)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
func DjbhashByte(data []byte, total uint64) uint64 {
	hash := uint64(5381)
	for _, v := range data {
		hash = (hash << 5) + uint64(v)
	}
	return hash % total
}
func FnvhashByte(data []byte, total uint64) uint64 {
	hash := uint64(2166136261)
	for _, v := range data {
		hash *= uint64(16777619)
		hash ^= uint64(v)
	}
	return hash % total
}
func DekhashByte(data []byte, total uint64) uint64 {
	hash := uint64(len(data))
	for _, v := range data {
		hash = ((hash << 5) ^ (hash >> 27)) ^ uint64(v)
	}
	return hash % total
}
func RshashByte(data []byte, total uint64) uint64 {
	seed := uint64(63689)
	hash := uint64(0)
	for _, v := range data {
		hash = hash*seed + uint64(v)
		seed *= uint64(378551)
	}
	return hash % total
}
func SdbmhashByte(data []byte, total uint64) uint64 {
	hash := uint64(0)
	for _, v := range data {
		hash = uint64(v) + (hash << 6) + (hash << 16) - hash
	}
	return hash % total
}
