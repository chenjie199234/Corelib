package msg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unsafe"
)

const (
	MSGONLINE  = 'a'
	MSGOFFLINE = 'b'
	MSGPULL    = 'c'
	MSGPUSH    = 'd'
	MSGREG     = 'r'
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
func GetOnlineMsg(data []byte) (string, *RegMsg, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil, nil
	}
	msg := new(RegMsg)
	datas := bytes.Split(data[1:], []byte{SPLIT})
	if len(datas) != 3 {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.getOnlineMsg]error:format unknown")
	}
	if e := json.Unmarshal(datas[1], &msg); e != nil {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]error:%s", e)
	}
	return Byte2str(datas[0]), msg, datas[2], nil
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
	datas := bytes.Split(data[1:], []byte{SPLIT})
	if len(datas) != 2 {
		return "", nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]error:format unknown")
	}
	return Byte2str(datas[0]), datas[1], nil
}
func MakePullMsg() []byte {
	return []byte{MSGPULL}
}
func MakePushMsg(data map[string]*RegMsg) []byte {
	d, _ := json.Marshal(data)
	return append([]byte{MSGPUSH}, d...)
}
func GetPushMsg(data []byte) (map[string]*RegMsg, error) {
	if len(data) <= 1 {
		return nil, nil
	}
	result := make(map[string]*RegMsg)
	if e := json.Unmarshal(data[1:], &result); e != nil {
		return nil, fmt.Errorf("[Discovery.msg.GetPushMsg]error:%s", e)
	}
	return result, nil
}
func MakeRegMsg(data *RegMsg) []byte {
	d, _ := json.Marshal(data)
	return append([]byte{MSGREG}, d...)
}
func GetRegMsg(data []byte) (*RegMsg, error) {
	msg := new(RegMsg)
	if e := json.Unmarshal(data[1:], &msg); e != nil {
		return nil, fmt.Errorf("[Discovery.msg.GetRegMsg]error:%s", e)
	}
	return msg, nil
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
