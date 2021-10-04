package stream

import (
	"context"
	"encoding/binary"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/util/common"
)

type Msg struct {
	ctx   context.Context
	send  bool
	mtype int
	data  []byte
	sid   int64
}

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

func getPingMsg(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrMsgEmpty
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

func getPongMsg(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrMsgEmpty
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
//9  |------------------------------------sid|
//10 |---------------------------------------|
//11 |---------------------------------------|
//12 |---------------------------------------|
//13 |---------------------------maxmsglength|
//...|---------------------------------------|
//x  |---------------------------------sender|
//...|---------------------------------------|
//y  |--------------------------specific data|
func makeVerifyMsg(sender string, verifydata []byte, sid int64, maxmsglength uint32) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + 8 + 4 + len(sender) + len(verifydata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+4+len(sender)+len(verifydata)))
	buf.Bytes()[4] = byte((VERIFY << 5) | len(sender))
	binary.BigEndian.PutUint64(buf.Bytes()[5:13], uint64(sid))
	binary.BigEndian.PutUint32(buf.Bytes()[13:17], maxmsglength)
	copy(buf.Bytes()[17:], sender)
	if len(verifydata) > 0 {
		copy(buf.Bytes()[17+len(sender):], verifydata)
	}
	return buf
}

func getVerifyMsg(data []byte) (string, []byte, int64, uint32, error) {
	senderlen := int(data[0] ^ (VERIFY << 5))
	if len(data) < (13 + senderlen) {
		return "", nil, 0, 0, ErrMsgEmpty
	}
	return common.Byte2str(data[13 : 13+senderlen]), data[13+senderlen:], int64(binary.BigEndian.Uint64(data[1:9])), binary.BigEndian.Uint32(data[9:13]), nil
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
//9  |------------------------------------sid|
//...|---------------------------------------|
//x  |--------------------------specific data|
func makeUserMsg(userdata []byte, sid int64) *bufpool.Buffer {
	buf := bufpool.GetBuffer()
	buf.Resize(uint32(4 + 1 + 8 + len(userdata)))
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(1+8+len(userdata)))
	buf.Bytes()[4] = byte(USER << 5)
	binary.BigEndian.PutUint64(buf.Bytes()[5:13], uint64(sid))
	if len(userdata) > 0 {
		copy(buf.Bytes()[13:], userdata)
	}
	return buf
}

func getUserMsg(data []byte) ([]byte, int64, error) {
	if len(data) <= 9 {
		return nil, 0, ErrMsgEmpty
	}
	return data[9:], int64(binary.BigEndian.Uint64(data[1:9])), nil
}

func getMsgType(data []byte) (int, error) {
	if len(data) == 0 {
		return -1, ErrMsgEmpty
	}
	t := int(data[0] >> 5)
	if t != PING && t != PONG && t != VERIFY && t != USER {
		//if t != PING && t != PONG && t != VERIFY && t != USER {
		return -1, ErrMsgUnknown
	}
	if t == VERIFY {
		senderlen := int(data[0] ^ (VERIFY << 5))
		if len(data) < (9 + senderlen) {
			return -1, ErrMsgEmpty
		}
	}
	if t == USER {
		if len(data) <= 9 {
			return -1, ErrMsgEmpty
		}
	}
	return t, nil
}
