package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	MSGONLINE  = 'a'
	MSGOFFLINE = 'b'
	MSGPULL    = 'c'
	MSGPUSH    = 'd'
	MSGREG     = 'r'
	SPLIT      = '|'
)

func makeOnlineMsg(peeruniquename string, data []byte, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(data)+len(hash)+3)
	result[0] = MSGONLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[len(peeruniquename)+1] = SPLIT
	copy(result[len(peeruniquename)+2:len(peeruniquename)+2+len(data)], data)
	result[len(peeruniquename)+2+len(data)] = SPLIT
	copy(result[len(peeruniquename)+len(data)+3:], hash)
	return result
}
func getOnlineMsg(data []byte) (string, *RegMsg, []byte, error) {
	if len(data) <= 1 {
		return "", nil, nil, nil
	}
	msg := new(RegMsg)
	datas := bytes.Split(data[1:], []byte{SPLIT})
	if len(datas) != 3 {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.getOnlineMsg]message broken,error:format unknown")
	}
	if e := json.Unmarshal(datas[1], &msg); e != nil {
		return "", nil, nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]message broken,error:%s", e)
	}
	return byte2str(datas[0]), msg, datas[2], nil
}
func makeOfflineMsg(peeruniquename string, hash []byte) []byte {
	result := make([]byte, len(peeruniquename)+len(hash)+2)
	result[0] = MSGOFFLINE
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	result[1+len(peeruniquename)] = SPLIT
	copy(result[len(peeruniquename)+2:], hash)
	return result
}
func getOfflineMsg(data []byte) ([]byte, []byte, error) {
	if len(data) <= 1 {
		return nil, nil, nil
	}
	datas := bytes.Split(data[1:], []byte{SPLIT})
	if len(datas) != 2 {
		return nil, nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]message broken,error:format unknown")
	}
	return datas[0], datas[1], nil
}
func makePullMsg() []byte {
	return []byte{MSGPULL}
}
func makePushMsg(data map[string]*RegMsg) []byte {
	d, _ := json.Marshal(data)
	return append([]byte{MSGPUSH}, d...)
}
func getPushMsg(data []byte) (map[string]*RegMsg, error) {
	if len(data) <= 1 {
		return nil, nil
	}
	result := make(map[string]*RegMsg)
	if e := json.Unmarshal(data[1:], &result); e != nil {
		return nil, fmt.Errorf("[Discovery.msg.GetPushMsg]message broken,error:%s", e)
	}
	return result, nil
}
func makeRegMsg(data *RegMsg) []byte {
	d, _ := json.Marshal(data)
	return append([]byte{MSGREG}, d...)
}
func getRegMsg(data []byte) (*RegMsg, error) {
	msg := new(RegMsg)
	if e := json.Unmarshal(data[1:], &msg); e != nil {
		return nil, fmt.Errorf("[Discovery.msg.GetRegMsg]message broken,error:%s", e)
	}
	return msg, nil
}
