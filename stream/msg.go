package stream

import (
	"encoding/binary"
	"errors"

	"github.com/chenjie199234/Corelib/pool"
)

var (
	ErrMsgEmpty   = errors.New("message empty")
	ErrMsgUnknown = errors.New("message type unknown")
	ErrMsgFin     = errors.New("message fin error")
)

const maxPieceLen = 65500

//msg type
const (
	_PING   = iota //can only be fin
	_PONG          //can only be fin
	_VERIFY        //can only be fin
	_USER          //can be fin or unfin
)

var typemask = 0b00000011

//control type
const (
	_EMPTY = iota << 2
)

//fin
var finmask = 0b10000000

func decodeHeader(data byte) (fin bool, mtype int, e error) {
	if int(data)&finmask > 0 {
		fin = true
	}
	mtype = int(data) & typemask
	switch mtype {
	case _PING:
		if !fin {
			return false, 0, ErrMsgFin
		}
	case _PONG:
		if !fin {
			return false, 0, ErrMsgFin
		}
	case _VERIFY:
		if !fin {
			return false, 0, ErrMsgFin
		}
	case _USER:
	default:
		e = ErrMsgUnknown
	}
	return
}

//   each row is one byte
//   |8    |7   |6    |5   |4   |3   |2   |1  |
//0  |----------------------------------------|
//1  |-----------------------------package len|
//2  |-fin-|-------------------------|--type--|
//...|----------------------------------------|
//x  |---------------------------specific data|
func makePingMsg(pingdata []byte) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.Resize(uint32(2 + 1 + len(pingdata)))
	binary.BigEndian.PutUint16(buf.Bytes(), uint16(1+len(pingdata)))
	buf.Bytes()[2] = byte(_PING | finmask)
	if len(pingdata) > 0 {
		copy(buf.Bytes()[3:], pingdata)
	}
	return buf
}

//   each row is one byte
//   |8    |7   |6    |5   |4   |3   |2   |1  |
//0  |----------------------------------------|
//1  |-----------------------------package len|
//2  |-fin-|-------------------------|--type--|
//...|----------------------------------------|
//x  |---------------------------specific data|
func makePongMsg(pongdata []byte) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.Resize(uint32(2 + 1 + len(pongdata)))
	binary.BigEndian.PutUint16(buf.Bytes(), uint16(1+len(pongdata)))
	buf.Bytes()[2] = byte(_PONG | finmask)
	if len(pongdata) > 0 {
		copy(buf.Bytes()[3:], pongdata)
	}
	return buf
}

//   each row is one byte
//   |8    |7   |6    |5   |4   |3   |2   |1  |
//0  |----------------------------------------|
//1  |-----------------------------package len|
//2  |-fin-|-------------------------|--type--|
//3  |----------------------------------------|
//4  |----------------------------------------|
//5  |----------------------------------------|
//6  |-------------------------------maxmsglen|
//7  |------------------------------sender len|
//...|----------------------------------------|
//x  |----------------------------------sender|
//...|----------------------------------------|
//y  |-----------------------------verify data|
func makeVerifyMsg(sender string, maxmsglen uint32, verifydata []byte) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.Resize(uint32(2 + 1 + 4 + 1 + len(sender) + len(verifydata)))
	binary.BigEndian.PutUint16(buf.Bytes(), uint16(1+4+1+len(sender)+len(verifydata)))
	buf.Bytes()[2] = byte(_VERIFY | finmask)
	binary.BigEndian.PutUint32(buf.Bytes()[3:], maxmsglen)
	buf.Bytes()[7] = byte(len(sender))
	copy(buf.Bytes()[8:], sender)
	if len(verifydata) > 0 {
		copy(buf.Bytes()[8+len(sender):], verifydata)
	}
	return buf
}

//'data' doesn't contain package len and head
func getVerifyMsg(data []byte) (sender string, maxmsglen uint32, verifydata []byte, e error) {
	if len(data) <= 5 {
		e = ErrMsgEmpty
		return
	}
	maxmsglen = binary.BigEndian.Uint32(data[:4])
	senderlen := int(data[4])
	if len(data) < (senderlen + 5) {
		e = ErrMsgEmpty
		return
	}
	sender = string(data[5 : senderlen+5])
	verifydata = data[senderlen+5:]
	return
}

//   each row is one byte
//   |8    |7   |6    |5   |4   |3   |2   |1  |
//0  |----------------------------------------|
//1  |-----------------------------package len|
//2  |-fin-|-------------------------|--type--|
//...|----------------------------------------|
//x  |-------------------------------user data|
func makeUserMsg(userdata []byte, fin bool) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.Resize(uint32(2 + 1 + len(userdata)))
	binary.BigEndian.PutUint16(buf.Bytes(), uint16(1+len(userdata)))
	if fin {
		buf.Bytes()[2] = byte(_USER | finmask)
	} else {
		buf.Bytes()[2] = byte(_USER)
	}
	if len(userdata) > 0 {
		copy(buf.Bytes()[3:], userdata)
	}
	return buf
}
