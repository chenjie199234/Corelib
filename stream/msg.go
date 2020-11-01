package stream

import (
	"encoding/binary"
	"fmt"

	"github.com/chenjie199234/Corelib/common"
)

const (
	HEART = iota
	VERIFY
	USER
)

//   each row is one byte
//   |8      |7   |6   |5   |4   |3   |2   |1   |
//1  |--------type|-----------------------------|
func makeHeartMsg(needprefix bool) []byte {
	data := []byte{byte(HEART << 6)}
	if needprefix {
		return addPrefix(data)
	}
	return data
}

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
//9  |---------------------------------starttime|
//...|------------------------------------------|
//x  |------------------------------------sender|
//...|------------------------------------------|
//y  |-----------------------------specific data|
func makeVerifyMsg(sender string, verifydata []byte, starttime uint64, needprefix bool) []byte {
	data := make([]byte, 9+len(sender)+len(verifydata))
	data[0] = byte((VERIFY << 6) | len(sender))
	binary.BigEndian.PutUint64(data[1:9], starttime)
	copy(data[9:], sender)
	if len(verifydata) > 0 {
		copy(data[9+len(sender):], verifydata)
	}
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func getVerifyMsg(data []byte) (string, []byte, uint64, error) {
	senderlen := int(data[0] ^ (VERIFY << 6))
	if len(data) < (9 + senderlen) {
		return "", nil, 0, fmt.Errorf("empty message")
	}

	return common.Byte2str(data[9 : 9+senderlen]), data[9+senderlen:], binary.BigEndian.Uint64(data[1:9]), nil
}

//   each row is one byte
//   |8      |7   |6   |5   |4   |3   |2   |1   |
//1  |--------type|-----------------------------|
//2  |------------------------------------------|
//3  |------------------------------------------|
//4  |------------------------------------------|
//5  |------------------------------------------|
//6  |------------------------------------------|
//7  |------------------------------------------|
//8  |------------------------------------------|
//9  |---------------------------------starttime|
//...|------------------------------------------|
//y  |-----------------------------specific data|
func makeUserMsg(userdata []byte, starttime uint64, needprefix bool) []byte {
	data := make([]byte, 9+len(userdata))
	data[0] = byte(USER << 6)
	binary.BigEndian.PutUint64(data[1:9], starttime)
	copy(data[9:], userdata)
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func getUserMsg(data []byte) ([]byte, uint64, error) {
	if len(data) < 9 {
		return nil, 0, fmt.Errorf("empty message")
	}
	return data[9:], binary.BigEndian.Uint64(data[1:9]), nil
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
