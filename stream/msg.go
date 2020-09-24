package stream

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

//   each row is one byte
//   |8      |7   |6   |5   |4   |3   |2   |1   |
//1  |--------type|----------------sender length|
//2  |------------------------------------------|
//3  |------------------------------------------|
//4  |------------------------------------------|
//5  |------------------------------------------|
//6  |------------------------------------------|
//7  |------------------------------------------|
//8  |------------------------------------------|
//9  |----------------------------------uniqueid|
//10 |------------------------------------------|
//...|------------------------------------sender|
//x  |------------------------------------------|
//...|-----------------------------specific data|

const (
	HEART = iota
	VERIFY
	USER
)

type heartMsg struct {
	uniqueid  uint64
	sender    string
	timestamp uint64
}
type verifyMsg struct {
	uniqueid   uint64
	sender     string
	verifydata []byte
}
type userMsg struct {
	uniqueid uint64
	sender   string
	userdata []byte
}

func makeHeartMsg(msg *heartMsg, needprefix bool) []byte {
	data := make([]byte, 9+len(msg.sender)+8)
	data[0] = byte((HEART << 6) | len(msg.sender))
	binary.BigEndian.PutUint64(data[1:9], msg.uniqueid)
	copy(data[9:], msg.sender)
	binary.BigEndian.PutUint64(data[9+len(msg.sender):], msg.timestamp)
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func getHeartMsg(data []byte) (*heartMsg, error) {
	senderlen := int(data[0] ^ (HEART << 6))
	if len(data) != (9 + senderlen + 8) {
		return nil, fmt.Errorf("bad heart message")
	}
	msg := &heartMsg{}
	msg.uniqueid = binary.BigEndian.Uint64(data[1:9])
	msg.sender = byte2str(data[9 : 9+senderlen])
	msg.timestamp = binary.BigEndian.Uint64(data[9+senderlen:])
	return msg, nil
}
func makeVerifyMsg(msg *verifyMsg, needprefix bool) []byte {
	data := make([]byte, 9+len(msg.sender)+len(msg.verifydata))
	data[0] = byte((VERIFY << 6) | len(msg.sender))
	binary.BigEndian.PutUint64(data[1:9], msg.uniqueid)
	copy(data[9:], msg.sender)
	if len(msg.verifydata) > 0 {
		copy(data[9+len(msg.sender):], msg.verifydata)
	}
	if needprefix {
		return addPrefix(data)
	}
	return data

}
func getVerifyMsg(data []byte) (*verifyMsg, error) {
	senderlen := int(data[0] ^ (VERIFY << 6))
	if len(data) < (9 + senderlen) {
		return nil, fmt.Errorf("bad verify message")
	}
	msg := &verifyMsg{}
	msg.uniqueid = binary.BigEndian.Uint64(data[1:9])
	msg.sender = byte2str(data[9 : 9+senderlen])
	msg.verifydata = data[9+senderlen:]
	return msg, nil
}
func makeUserMsg(msg *userMsg, needprefix bool) []byte {
	data := make([]byte, 9+len(msg.sender)+len(msg.userdata))
	data[0] = byte((USER << 6) | len(msg.sender))
	binary.BigEndian.PutUint64(data[1:9], msg.uniqueid)
	copy(data[9:], msg.sender)
	copy(data[9+len(msg.sender):], msg.userdata)
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func getUserMsg(data []byte) (*userMsg, error) {
	senderlen := int(data[0] ^ (USER << 6))
	if len(data) < (9 + senderlen + 1) {
		return nil, fmt.Errorf("bad user message")
	}
	msg := &userMsg{}
	msg.uniqueid = binary.BigEndian.Uint64(data[1:9])
	msg.sender = byte2str(data[9 : 9+senderlen])
	msg.userdata = data[9+senderlen:]
	return msg, nil
}
func getMsgType(data []byte) (int, error) {
	if len(data) == 0 {
		return -1, fmt.Errorf("empty message")
	}
	switch {
	case (data[0] >> 6) == HEART:
		return HEART, nil
	case (data[0] >> 6) == VERIFY:
		return VERIFY, nil
	case (data[0] >> 6) == USER:
		return USER, nil
	default:
		return -1, fmt.Errorf("unknown message type")
	}
}
func addPrefix(data []byte) []byte {
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(data)))
	return append(prefix, data...)
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
