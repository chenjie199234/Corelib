package discovery

import (
	"bytes"
	"fmt"
	"unsafe"
)

const (
	mSGONLINE  = 'a'
	mSGOFFLINE = 'b'
	mSGPULL    = 'c'
	mSGPUSH    = 'd'
	sPLIT      = '|'
)

type RegMsg struct {
	GrpcIp      string `json:"gi,omitempty"`
	GrpcPort    int    `json:"gp,omitempty"`
	HttpIp      string `json:"hi,omitempty"`
	HttpPort    int    `json:"hp,omitempty"`
	TcpIp       string `json:"ti,omitempty"`
	TcpPort     int    `json:"tp,omitempty"`
	WebSockIp   string `json:"wi,omitempty"`
	WebSockPort int    `json:"wp,omitempty"`
}
type NoticeMsg struct {
	PeerAddr        string `json:"p"` //peer's addr
	Status          bool   `json:"s"` //true-online,false-offline
	DiscoveryServer string `json:"d"` //happened on which discovery server
}

func makeOnlineMsg(peeruniquename string, data []byte, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(data)+len(hash)+3)
	result[0] = mSGONLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[len(peeruniquename)+1] = sPLIT
	copy(result[len(peeruniquename)+2:len(peeruniquename)+2+len(data)], data)
	result[len(peeruniquename)+2+len(data)] = sPLIT
	copy(result[len(peeruniquename)+len(data)+3:], hash)
	return result
}
func getOnlineMsg(data []byte) (string, []byte, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil, nil
	}
	if bytes.Count(data, []byte{sPLIT}) < 2 {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.getOnlineMsg]error:format unknwon")
	}
	firstindex := bytes.Index(data, []byte{sPLIT})
	secondindex := bytes.Index(data[firstindex+1:], []byte{sPLIT}) + firstindex + 1
	return byte2str(data[1:firstindex]), data[firstindex+1 : secondindex], data[secondindex+1:], nil
}
func makeOfflineMsg(peeruniquename string, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(hash)+2)
	result[0] = mSGOFFLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[1+len(peeruniquename)] = sPLIT
	copy(result[len(peeruniquename)+2:], hash)
	return result
}
func getOfflineMsg(data []byte) (string, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil
	}
	if bytes.Count(data, []byte{sPLIT}) < 1 {
		return "", nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]error:format unknown")
	}
	index := bytes.Index(data, []byte{sPLIT})
	return byte2str(data[1:index]), data[index+1:], nil
}
func makePullMsg() []byte {
	return []byte{mSGPULL}
}
func makePushMsg(data map[string][]byte) []byte {
	count := 0
	for k, v := range data {
		count += len(k) + 1
		count += len(v) + 1
	}
	if count == 0 {
		return []byte{mSGPUSH}
	}
	result := make([]byte, count)
	index := 0
	for k, v := range data {
		if index == 0 {
			result[index] = mSGPUSH
		} else {
			result[index] = sPLIT
		}
		index++
		copy(result[index:len(k)+index], k)
		index += len(k)
		result[index] = sPLIT
		index++
		copy(result[index:len(v)+index], v)
		index += len(v)
	}
	return result
}
func getPushMsg(data []byte) (map[string][]byte, error) {
	if len(data) <= 1 {
		return nil, nil
	}
	datas := bytes.Split(data[1:], []byte{sPLIT})
	if len(datas)%2 != 0 {
		return nil, fmt.Errorf("[Discovery.msg.GetPushMsg]error:format unknown")
	}
	result := make(map[string][]byte, int(float64(len(datas))*1.3))
	for i := 0; i < len(datas); i += 2 {
		result[byte2str(datas[i])] = datas[i+1]
	}
	return result, nil
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
func bkdrhash(peeruniquename string, total uint64) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range peeruniquename {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
