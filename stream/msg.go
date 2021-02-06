package stream

import (
	"encoding/binary"
	"fmt"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/common"
)

const (
	HEART = iota
	VERIFY
	USER
	CLOSEREAD
	CLOSEWRITE
)

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-------type---|------------------------|
func makeHeartMsg(needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	if needprefix {
		buf.Grow(5)
		binary.BigEndian.PutUint32(buf.Bytes(), 1)
	}
	data := buf.Bytes()[4:]
	data[0] = byte(HEART << 5)
	return buf
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|-----------sender length|
//2  |---------------------------------------|
//3  |---------------------------------------|
//4  |---------------------------------------|
//5  |---------------------------------------|
//6  |---------------------------------------|
//7  |---------------------------------------|
//8  |---------------------------------------|
//9  |------------------------------starttime|
//...|---------------------------------------|
//x  |---------------------------------sender|
//...|---------------------------------------|
//y  |--------------------------specific data|
func makeVerifyMsg(sender string, verifydata []byte, starttime uint64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	if needprefix {
		buf.Grow(4 + 1 + 8 + len(sender) + len(verifydata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+len(sender)+len(verifydata)))
	}
	data := buf.Bytes()[4:]
	data[0] = byte((VERIFY << 5) | len(sender))
	binary.BigEndian.PutUint64(data[1:9], starttime)
	copy(data[9:], sender)
	if len(verifydata) > 0 {
		copy(data[9+len(sender):], verifydata)
	}
	return buf
}
func getVerifyMsg(data []byte) (string, []byte, uint64, error) {
	senderlen := int(data[0] ^ (VERIFY << 5))
	if len(data) < (9 + senderlen) {
		return "", nil, 0, fmt.Errorf("empty message")
	}
	return common.Byte2str(data[9 : 9+senderlen]), data[9+senderlen:], binary.BigEndian.Uint64(data[1:9]), nil
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|------------------------|
//2  |---------------------------------------|
//3  |---------------------------------------|
//4  |---------------------------------------|
//5  |---------------------------------------|
//6  |---------------------------------------|
//7  |---------------------------------------|
//8  |---------------------------------------|
//9  |------------------------------starttime|
//...|---------------------------------------|
//y  |--------------------------specific data|
func makeUserMsg(userdata []byte, starttime uint64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	if needprefix {
		buf.Grow(4 + 1 + 8 + len(userdata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+len(userdata)))
	}
	data := buf.Bytes()[4:]
	data[0] = byte(USER << 5)
	binary.BigEndian.PutUint64(data[1:9], starttime)
	if len(userdata) > 0 {
		copy(data[9:], userdata)
	}
	return buf
}
func getUserMsg(data []byte) ([]byte, uint64, error) {
	if len(data) <= 9 {
		return nil, 0, fmt.Errorf("empty message")
	}
	return data[9:], binary.BigEndian.Uint64(data[1:9]), nil
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|------------------------|
//2  |---------------------------------------|
//3  |---------------------------------------|
//4  |---------------------------------------|
//5  |---------------------------------------|
//6  |---------------------------------------|
//7  |---------------------------------------|
//8  |---------------------------------------|
//9  |------------------------------starttime|
func makeCloseReadMsg(starttime uint64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	if needprefix {
		buf.Grow(4 + 1 + 8)
		binary.BigEndian.PutUint32(buf.Bytes(), 1+8)
	}
	data := buf.Bytes()[4:]
	data[0] = byte(CLOSEREAD << 5)
	binary.BigEndian.PutUint64(data[1:9], starttime)
	return buf
}
func getCloseReadMsg(data []byte) (uint64, error) {
	if len(data) < 9 {
		return 0, fmt.Errorf("empty message")
	}
	return binary.BigEndian.Uint64(data[1:9]), nil
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|------------------------|
//2  |---------------------------------------|
//3  |---------------------------------------|
//4  |---------------------------------------|
//5  |---------------------------------------|
//6  |---------------------------------------|
//7  |---------------------------------------|
//8  |---------------------------------------|
//9  |------------------------------starttime|
func makeCloseWriteMsg(starttime uint64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	if needprefix {
		buf.Grow(4 + 1 + 8)
		binary.BigEndian.PutUint32(buf.Bytes(), 1+8)
	}
	data := buf.Bytes()[4:]
	data[0] = byte(CLOSEWRITE << 5)
	binary.BigEndian.PutUint64(data[1:9], starttime)
	return buf
}
func getCloseWriteMsg(data []byte) (uint64, error) {
	if len(data) < 9 {
		return 0, fmt.Errorf("empty message")
	}
	return binary.BigEndian.Uint64(data[1:9]), nil
}
func getMsgType(data []byte) (int, error) {
	if len(data) == 0 {
		return -1, fmt.Errorf("empty message")
	}
	switch {
	case (data[0] >> 5) == HEART:
		return HEART, nil
	case (data[0] >> 5) == VERIFY:
		senderlen := int(data[0] ^ (VERIFY << 5))
		if len(data) < (9 + senderlen) {
			return -1, fmt.Errorf("empty message")
		}
		return VERIFY, nil
	case (data[0] >> 5) == USER:
		if len(data) <= 9 {
			return -1, fmt.Errorf("empty message")
		}
		return USER, nil
	case (data[0] >> 5) == CLOSEREAD:
		if len(data) < 9 {
			return -1, fmt.Errorf("empty message")
		}
		return CLOSEREAD, nil
	case (data[0] >> 5) == CLOSEWRITE:
		if len(data) < 9 {
			return -1, fmt.Errorf("empty message")
		}
		return CLOSEWRITE, nil
	default:
		return -1, fmt.Errorf("unknown message type")
	}
}
