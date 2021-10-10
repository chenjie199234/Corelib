package stream

import (
	"encoding/binary"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	PING = iota + 1
	PONG
	VERIFY
	USER
)

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-------type---|------------------------|
//...|---------------------------------------|
//x  |--------------------------specific data|
func makePingMsg(pingdata []byte) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + len(pingdata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+len(pingdata)))
	buf.Bytes()[4] = byte(PING << 5)
	if len(pingdata) > 0 {
		copy(buf.Bytes()[5:], pingdata)
	}
	return buf
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-------type---|------------------------|
//...|---------------------------------------|
//x  |--------------------------specific data|
func makePongMsg(pongdata []byte) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + len(pongdata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+len(pongdata)))
	buf.Bytes()[4] = byte(PONG << 5)
	if len(pongdata) > 0 {
		copy(buf.Bytes()[5:], pongdata)
	}
	return buf
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|-----------sender length|
//2  |---------------------------------------|
//3  |---------------------------------------|
//4  |---------------------------------------|
//5  |---------------------------maxmsglength|
//...|---------------------------------------|
//x  |---------------------------------sender|
//...|---------------------------------------|
//y  |----------------------------verify data|
func makeVerifyMsg(sender string, verifydata []byte, maxmsglength uint32) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + 4 + len(sender) + len(verifydata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+4+len(sender)+len(verifydata)))
	buf.Bytes()[4] = byte((VERIFY << 5) | len(sender))
	binary.BigEndian.PutUint32(buf.Bytes()[5:9], maxmsglength)
	copy(buf.Bytes()[9:], sender)
	if len(verifydata) > 0 {
		copy(buf.Bytes()[9+len(sender):], verifydata)
	}
	return buf
}

//   each row is one byte
//   |8   |7   |6   |5   |4   |3   |2   |1   |
//1  |-----type-----|------------------------|
//...|---------------------------------------|
//x  |------------------------------user data|
func makeUserMsg(userdata []byte) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + len(userdata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+len(userdata)))
	buf.Bytes()[4] = byte(USER << 5)
	if len(userdata) > 0 {
		copy(buf.Bytes()[5:], userdata)
	}
	return buf
}

func decodeMsg(data []byte) (mtype int, specificdata []byte, sender string, maxmsglength uint32, e error) {
	if len(data) == 0 {
		e = ErrMsgEmpty
		return
	}
	switch data[0] >> 5 {
	case PING:
		mtype = PING
		specificdata = data[1:]
	case PONG:
		mtype = PONG
		specificdata = data[1:]
	case VERIFY:
		mtype = VERIFY
		senderlen := int(data[0] ^ (VERIFY << 5))
		if len(data) < (5 + senderlen) {
			e = ErrMsgEmpty
		}
		sender = common.Byte2str(data[5 : 5+senderlen])
		specificdata = data[5+senderlen:]
		maxmsglength = binary.BigEndian.Uint32(data[1:5])
	case USER:
		mtype = USER
		if len(data) == 1 {
			e = ErrMsgEmpty
		} else {
			specificdata = data[1:]
		}
	default:
		e = ErrMsgUnknown
	}
	return
}
