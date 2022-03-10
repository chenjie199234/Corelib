package stream

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/chenjie199234/Corelib/pool"
)

var (
	ErrMsgLarge = errors.New("message too large")
	ErrMsgFin   = errors.New("message fin error")
	ErrMsgType  = errors.New("message type error")
	ErrMsgMask  = errors.New("message mask error")
)

// use websocket frame format,so that we can support raw tcp and websocket at the same time
// when use raw tcp,the client don't need to use mask and don't need websocket handshake
// when use the websocket,the client must use mask and need http handshake
// after connection success,first message must be verify message and must be fin,verify message data format see below

// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-------------------------------+
// |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
// |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
// |N|V|V|V|       |S|             |   (if payload len==126/127)   |
// | |1|2|3|       |K|             |                               |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
// |     Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-------------------------------+
// |                               |Masking-key, if MASK set to 1  |
// +-------------------------------+-------------------------------+
// | Masking-key (continued)       |          Payload Data         |
// +-------------------------------- - - - - - - - - - - - - - - - +
// :                     Payload Data continued ...                :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...                |
// +---------------------------------------------------------------+

var maxPieceLen = 65500

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	_CONTINUE = 0b00000000 //can be not fin
	_TEXT     = 0b00000001 //can be not fin
	_BINARY   = 0b00000010 //can be not fin
	_CLOSE    = 0b00001000 //must be fin,payload <= 125
	_PING     = 0b00001001 //must be fin,payload <= 125
	_PONG     = 0b00001010 //must be fin,payload <= 125
	_RSV3     = 0b00010000
	_RSV2     = 0b00100000
	_RSV1     = 0b01000000
	_FIN_MASK = 0b10000000
)

func decodeFirst(b byte) (fin, rsv1, rsv2, rsv3 bool, opcode int, e error) {
	if b&_FIN_MASK > 0 {
		fin = true
	}
	if b&_RSV1 > 0 {
		rsv1 = true
	}
	if b&_RSV2 > 0 {
		rsv2 = true
	}
	if b&_RSV3 > 0 {
		rsv3 = true
	}
	opcode = int(b & 0b00001111)
	if opcode != _CONTINUE && opcode != _TEXT && opcode != _BINARY && opcode != _CLOSE && opcode != _PING && opcode != _PONG {
		e = ErrMsgType
		return
	}
	if iscontrol(opcode) && !fin {
		e = ErrMsgFin
	}
	return
}
func decodeSecond(b byte, opcode int, peerWSClient bool) (mask bool, payload uint32, e error) {
	if b&_FIN_MASK > 0 {
		mask = true
	}
	if !mask && peerWSClient {
		//websocket client's message must mask
		e = ErrMsgMask
		return
	}
	payload = uint32(b & 0b01111111)
	if iscontrol(opcode) && payload > 125 {
		e = ErrMsgLarge
		return
	}
	return
}
func iscontrol(opcode int) bool {
	return opcode&0b00001000 > 0
}

func makePingMsg(nowUnixnano int64, mask bool) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.AppendByte(_PING | _FIN_MASK)
	if mask {
		buf.AppendByte(_FIN_MASK | 8)
		buf.Growth(14)
		binary.BigEndian.PutUint32(buf.Bytes()[2:], rand.Uint32())
		binary.BigEndian.PutUint64(buf.Bytes()[6:], uint64(nowUnixnano))
		domask(buf.Bytes()[6:], buf.Bytes()[2:6])
	} else {
		buf.AppendByte(8)
		buf.Growth(10)
		binary.BigEndian.PutUint64(buf.Bytes()[2:], uint64(nowUnixnano))
	}
	return buf
}

func makePongMsg(pongdata []byte, mask bool) *pool.Buffer {
	buf := pool.GetBuffer()
	buf.AppendByte(_PONG | _FIN_MASK)
	if mask {
		buf.AppendByte(_FIN_MASK | byte(len(pongdata)))
		buf.Growth(6)
		binary.BigEndian.PutUint32(buf.Bytes()[2:], rand.Uint32())
		domask(pongdata, buf.Bytes()[2:])
		buf.AppendByteSlice(pongdata)
	} else {
		buf.AppendByte(byte(len(pongdata)))
		buf.AppendByteSlice(pongdata)
	}
	return buf
}

//length is always <= math.MaxUint32
func makeheader(buf *pool.Buffer, fin bool, piece int, mask bool, length uint64) (maskkey []byte) {
	if fin && piece == 0 {
		buf.AppendByte(_FIN_MASK | _BINARY)
	} else if fin {
		buf.AppendByte(_FIN_MASK | _CONTINUE)
	} else if piece != 0 {
		buf.AppendByte(_CONTINUE)
	} else {
		buf.AppendByte(_BINARY)
	}
	var payload uint8
	switch {
	case length <= 125:
		payload = uint8(length)
	case length <= math.MaxUint16:
		payload = 126
	default:
		payload = 127
	}
	if mask {
		buf.AppendByte(_FIN_MASK | payload)
	} else {
		buf.AppendByte(payload)
	}
	switch payload {
	case 127:
		buf.Growth(10)
		binary.BigEndian.PutUint64(buf.Bytes()[2:], length)
	case 126:
		buf.Growth(4)
		binary.BigEndian.PutUint16(buf.Bytes()[2:], uint16(length))
	}
	if mask {
		buf.Growth(uint32(buf.Len()) + 4)
		binary.BigEndian.PutUint32(buf.Bytes()[buf.Len()-4:], rand.Uint32())
		maskkey = buf.Bytes()[buf.Len()-4:]
	}
	return
}
func domask(data []byte, maskkey []byte) {
	for i, v := range data {
		data[i] = v ^ maskkey[i%4]
	}
}

//Payload Data
//  0 1 2 3 4 5 6 7 8
//0 |---------------|
//1 |---------------|
//2 |---------------|
//3 |------maxmsglen|//big endian
//  ...
//  :-----verifydata|
func makeVerifyMsg(maxmsglen uint32, verifydata []byte, mask bool) *pool.Buffer {
	payloadlen := 4 + len(verifydata)
	buf := pool.GetBuffer()
	maskkey := makeheader(buf, true, 0, mask, uint64(payloadlen))
	headlen := buf.Len()
	buf.Growth(uint32(headlen + payloadlen))
	binary.BigEndian.PutUint32(buf.Bytes()[headlen:], maxmsglen)
	copy(buf.Bytes()[headlen+4:], verifydata)
	if mask {
		domask(buf.Bytes()[headlen:], maskkey)
	}
	return buf
}

//return verifydata == nil --> format wrong
//return len(verifydata) == 0 --> no verifydata
func getVerifyMsg(data []byte) (maxmsglen uint32, verifydata []byte) {
	if len(data) < 4 {
		return
	}
	maxmsglen = binary.BigEndian.Uint32(data[:4])
	verifydata = data[4:]
	return
}

//Payload Data
//  0 1 2 3 4 5 6 7 8
//0 |---------------|
//  ...
//  :-----------data|
func makeCommonMsg(data []byte, fin bool, piece int, mask bool) *pool.Buffer {
	payloadlen := len(data)
	buf := pool.GetBuffer()
	maskkey := makeheader(buf, fin, piece, mask, uint64(payloadlen))
	headlen := buf.Len()
	buf.Growth(uint32(headlen + payloadlen))
	copy(buf.Bytes()[headlen:], data)
	if mask {
		domask(buf.Bytes()[headlen:], maskkey)
	}
	return buf
}
