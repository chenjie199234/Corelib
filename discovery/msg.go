package discovery

import (
	"bytes"
	"fmt"
)

const (
	MSGONLINE  = 'a'
	MSGOFFLINE = 'b'
	MSGPULL    = 'c'
	MSGPUSH    = 'd'
	SPLIT
)

func makeOnlineMsg(peernameip string, hash []byte) []byte {
	data := make([]byte, len(peernameip)+len(hash)+2)
	data[0] = MSGONLINE
	copy(data[1:len(peernameip)+1], peernameip)
	data[1+len(peernameip)] = SPLIT
	copy(data[1+len(peernameip)+1:], hash)
	return data
}
func getOnlineMsg(data []byte) ([]byte, []byte, error) {
	if len(data) <= 1 {
		return nil, nil, nil
	}
	result := bytes.Split(data[1:], []byte{SPLIT})
	if len(result) != 2 {
		return nil, nil, fmt.Errorf("[Discovery.msg.GetOnlineMsg]message broken")
	}
	return result[0], result[1], nil
}
func makeOfflineMsg(peernameip string, hash []byte) []byte {
	data := make([]byte, len(peernameip)+len(hash)+2)
	data[0] = MSGONLINE
	copy(data[1:len(peernameip)+1], peernameip)
	data[1+len(peernameip)] = SPLIT
	copy(data[1+len(peernameip)+1:], hash)
	return data
}
func getOfflineMsg(data []byte) ([]byte, []byte, error) {
	if len(data) <= 1 {
		return nil, nil, nil
	}
	result := bytes.Split(data[1:], []byte{SPLIT})
	if len(result) != 2 {
		return nil, nil, fmt.Errorf("[Discovery.msg.GetOfflineMsg]message broken")
	}
	return result[0], result[1], nil
}
func makePullMsg() []byte {
	data := []byte{MSGPULL}
	return data
}
func makePushMsg(peernameips []string) []byte {
	count := 1
	for i, peernameip := range peernameips {
		if i == 0 {
			count += len(peernameip)
		} else {
			count += (len(peernameip) + 1)
		}
	}
	data := make([]byte, count)
	data[0] = MSGPUSH
	count = 1
	for i, peernameip := range peernameips {
		if i == 0 {
			copy(data[count:len(peernameip)+count], peernameip)
			count += len(peernameip)
		} else {
			data[count] = SPLIT
			count++
			copy(data[count:len(peernameip)+count], peernameip)
			count += len(peernameip)
		}
	}
	return data
}
func getPushMsg(data []byte) [][]byte {
	if len(data) <= 1 {
		return nil
	}
	result := bytes.Split(data[1:], []byte{SPLIT})
	return result
}
