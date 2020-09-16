package msg

import (
	"bytes"
	"fmt"
	"unsafe"
)

const (
	MSGONLINE  = 'a'
	MSGOFFLINE = 'b'
	MSGPULL    = 'c'
	MSGPUSH    = 'd'
	SPLIT      = '|'
)

type RegMsg struct {
	GrpcAddr string `json:"g"`
	HttpAddr string `json:"h"`
	TcpAddr  string `json:"t"`
}

func MakeOnlineMsg(peeruniquename string, data []byte, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(data)+len(hash)+3)
	result[0] = MSGONLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[len(peeruniquename)+1] = SPLIT
	copy(result[len(peeruniquename)+2:len(peeruniquename)+2+len(data)], data)
	result[len(peeruniquename)+2+len(data)] = SPLIT
	copy(result[len(peeruniquename)+len(data)+3:], hash)
	return result
}
func GetOnlineMsg(data []byte) (string, []byte, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil, nil
	}
	if bytes.Count(data, []byte{SPLIT}) < 2 {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.getOnlineMsg]error:format unknwon")
	}
	firstindex := bytes.Index(data, []byte{SPLIT})
	secondindex := bytes.Index(data[firstindex+1:], []byte{SPLIT}) + firstindex + 1
	return Byte2str(data[1:firstindex]), data[firstindex+1 : secondindex], data[secondindex+1:], nil
}
func MakeOfflineMsg(peeruniquename string, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(hash)+2)
	result[0] = MSGOFFLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[1+len(peeruniquename)] = SPLIT
	copy(result[len(peeruniquename)+2:], hash)
	return result
}
func GetOfflineMsg(data []byte) (string, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil
	}
	if bytes.Count(data, []byte{SPLIT}) < 1 {
		return "", nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]error:format unknown")
	}
	index := bytes.Index(data, []byte{SPLIT})
	return Byte2str(data[1:index]), data[index+1:], nil
}
func MakePullMsg() []byte {
	return []byte{MSGPULL}
}
func MakePushMsg(data map[string][]byte) []byte {
	count := 0
	for k, v := range data {
		count += len(k) + 1
		count += len(v) + 1
	}
	result := make([]byte, count)
	index := 0
	for k, v := range data {
		if index == 0 {
			result[index] = MSGPUSH
		} else {
			result[index] = SPLIT
		}
		index++
		copy(result[index:len(k)+index], k)
		index += len(k)
		result[index] = SPLIT
		index++
		copy(result[index:len(v)+index], v)
		index += len(v)
	}
	return result
}
func GetPushMsg(data []byte) (map[string][]byte, error) {
	if len(data) <= 1 {
		return nil, nil
	}
	result := make(map[string][]byte, 10)
	var preindex int
	var k string
	for {
		index := bytes.Index(data[preindex+1:], []byte{SPLIT})
		if index == -1 {
			if k != "" {
				return nil, fmt.Errorf("[Discovery.msg.GetPushMsg]error:format unknown")
			}
			break
		}
		if k == "" {
			k = Byte2str(data[preindex+1 : index])
		} else {
			result[k] = data[preindex+1 : index]
			k = ""
		}
		preindex = index
	}
	return result, nil
}
func Str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func Byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
func Bkdrhash(peeruniquename string, total uint64) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range peeruniquename {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
