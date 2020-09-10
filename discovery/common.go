package discovery

import "unsafe"

func bkdrhash(nameip string, total uint64) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range nameip {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
