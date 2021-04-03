package stream

import (
	"encoding/binary"
	"errors"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	PING = iota + 1
	PONG
	VERIFY
	USER
	WSPING
	WSPONG
)

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-------type---|------------------------|
//...|---------------------------------------|
//x  |--------------------------specific data|
func makePingMsg(pingdata []byte, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	var data []byte
	if needprefix {
		buf.Grow(4 + 1 + len(pingdata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+len(pingdata)))
		data = buf.Bytes()[4:]
	} else {
		buf.Grow(1 + len(pingdata))
		data = buf.Bytes()
	}
	data[0] = byte(PING << 5)
	if len(pingdata) > 0 {
		copy(data[1:], pingdata)
	}
	return buf
}

func makeWsPingMsg(pingdata []byte) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Grow(1 + len(pingdata))
	buf.Bytes()[0] = byte(WSPING << 5)
	if len(pingdata) > 0 {
		copy(buf.Bytes()[1:], pingdata)
	}
	return buf
}

//data must without prefix
func getPingMsg(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty message")
	}
	if len(data) > 1 {
		return data[1:], nil
	}
	return nil, nil
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-------type---|------------------------|
//...|---------------------------------------|
//x  |--------------------------specific data|
func makePongMsg(pongdata []byte, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	var data []byte
	if needprefix {
		buf.Grow(4 + 1 + len(pongdata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+len(pongdata)))
		data = buf.Bytes()[4:]
	} else {
		buf.Grow(1 + len(pongdata))
		data = buf.Bytes()
	}
	data[0] = byte(PONG << 5)
	if len(pongdata) > 0 {
		copy(data[1:], pongdata)
	}
	return buf
}

func makeWsPongMsg(pongdata []byte) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Grow(1 + len(pongdata))
	buf.Bytes()[0] = byte(WSPONG << 5)
	if len(pongdata) > 0 {
		copy(buf.Bytes()[1:], pongdata)
	}
	return buf
}

//data must without prefix
func getPongMsg(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty message")
	}
	if len(data) > 1 {
		return data[1:], nil
	}
	return nil, nil
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
func makeVerifyMsg(sender string, verifydata []byte, starttime int64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	var data []byte
	if needprefix {
		buf.Grow(4 + 1 + 8 + len(sender) + len(verifydata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+len(sender)+len(verifydata)))
		data = buf.Bytes()[4:]
	} else {
		buf.Grow(1 + 8 + len(sender) + len(verifydata))
		data = buf.Bytes()
	}
	data[0] = byte((VERIFY << 5) | len(sender))
	binary.BigEndian.PutUint64(data[1:9], uint64(starttime))
	copy(data[9:], sender)
	if len(verifydata) > 0 {
		copy(data[9+len(sender):], verifydata)
	}
	return buf
}

//data must without prefix
func getVerifyMsg(data []byte) (string, []byte, int64, error) {
	senderlen := int(data[0] ^ (VERIFY << 5))
	if len(data) < (9 + senderlen) {
		return "", nil, 0, errors.New("empty message")
	}
	return common.Byte2str(data[9 : 9+senderlen]), data[9+senderlen:], int64(binary.BigEndian.Uint64(data[1:9])), nil
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
func makeUserMsg(userdata []byte, starttime int64, needprefix bool) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	var data []byte
	if needprefix {
		buf.Grow(4 + 1 + 8 + len(userdata))
		binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+len(userdata)))
		data = buf.Bytes()[4:]
	} else {
		buf.Grow(1 + 8 + len(userdata))
		data = buf.Bytes()
	}
	data[0] = byte(USER << 5)
	binary.BigEndian.PutUint64(data[1:9], uint64(starttime))
	if len(userdata) > 0 {
		copy(data[9:], userdata)
	}
	return buf
}

//data must without prefix
func getUserMsg(data []byte) ([]byte, int64, error) {
	if len(data) <= 9 {
		return nil, 0, errors.New("empty message")
	}
	return data[9:], int64(binary.BigEndian.Uint64(data[1:9])), nil
}

//data must without prefix
func getMsgType(data []byte) (int, error) {
	if len(data) == 0 {
		return -1, errors.New("empty message")
	}
	t := int(data[0] >> 5)
	if t != PING && t != PONG && t != WSPING && t != WSPONG && t != VERIFY && t != USER {
		//if t != PING && t != PONG && t != VERIFY && t != USER {
		return -1, errors.New("unknown message type")
	}
	if t == VERIFY {
		senderlen := int(data[0] ^ (VERIFY << 5))
		if len(data) < (9 + senderlen) {
			return -1, errors.New("empty message")
		}
	}
	if t == USER {
		if len(data) <= 9 {
			return -1, errors.New("empty message")
		}
	}
	return t, nil
}
