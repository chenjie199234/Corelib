package ws

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"net"

	"github.com/chenjie199234/Corelib/pool"
)

// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-----------------------------+
// |F|R|R|R| opcode|M| Payload len |  Extended payload length    |
// |I|S|S|S|  (4)  |A|     (7)     |           (16/64)           |
// |N|V|V|V|       |S|             |  if payload len==126/127)   |
// | |1|2|3|       |K|             |                             |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - +
// |   Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-----------------------------+
// |                               |Masking-key, if MASK set to 1|
// +-------------------------------+-----------------------------+
// | Masking-key (continued)       |          Payload Data       |
// +-------------------------------- - - - - - - - - - - - - - - +
// :                     Payload Data continued ...              :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...              |
// +-------------------------------------------------------------+

func makeheader(buf *pool.Buffer, fin, firstpiece, mask bool, length uint64, msgtype uint8) (maskkey []byte) {
	if fin && firstpiece {
		buf.AppendByte(_FIN_MASK | msgtype)
	} else if fin {
		buf.AppendByte(uint8(_FIN_MASK | _CONTINUE))
	} else if !firstpiece {
		buf.AppendByte(uint8(_CONTINUE))
	} else {
		buf.AppendByte(msgtype)
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
		binary.BigEndian.PutUint64(buf.Bytes()[2:], uint64(length))
	case 126:
		buf.Growth(4)
		binary.BigEndian.PutUint16(buf.Bytes()[2:], uint16(length))
	}
	if mask {
		buf.Growth(uint32(buf.Len()) + 4)
		rand.Read(buf.Bytes()[buf.Len()-4:])
		maskkey = buf.Bytes()[buf.Len()-4:]
	}
	return
}

func WriteMsg(conn net.Conn, data []byte, fin, firstpiece, mask bool) error {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	maskkey := makeheader(buf, fin, firstpiece, mask, uint64(len(data)), uint8(_BINARY))
	headlen := buf.Len()
	buf.AppendByteSlice(data)
	if mask {
		domask(buf.Bytes()[headlen:], maskkey)
	}
	if _, e := conn.Write(buf.Bytes()); e != nil {
		return e
	}
	return nil
}

// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
func WritePing(conn net.Conn, data []byte, mask bool) error {
	if len(data) > 125 {
		return ErrMsgLarge
	}
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	maskkey := makeheader(buf, true, true, mask, uint64(len(data)), uint8(_PING))
	headlen := buf.Len()
	buf.AppendByteSlice(data)
	if mask {
		domask(buf.Bytes()[headlen:], maskkey)
	}
	if _, e := conn.Write(buf.Bytes()); e != nil {
		return e
	}
	return nil
}

// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
func WritePong(conn net.Conn, data []byte, mask bool) error {
	if len(data) > 125 {
		return ErrMsgLarge
	}
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	maskkey := makeheader(buf, true, true, mask, uint64(len(data)), uint8(_PONG))
	headlen := buf.Len()
	buf.AppendByteSlice(data)
	if mask {
		domask(buf.Bytes()[headlen:], maskkey)
	}
	if _, e := conn.Write(buf.Bytes()); e != nil {
		return e
	}
	return nil
}
