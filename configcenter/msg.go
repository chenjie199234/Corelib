package configcenter

const (
	msglistapp = 'a'
	msgsetapp  = 'b'
	msggetapp  = 'c'
	msgpushapp = 'd'
)

func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
