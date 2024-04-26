package ws

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"net"

	"github.com/chenjie199234/Corelib/pool/bpool"
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

func makeheader(buf *[]byte, fin, firstpiece, mask bool, length uint64, msgtype uint8) (maskkey []byte) {
	if fin && firstpiece {
		*buf = append(*buf, _FIN|msgtype)
	} else if fin {
		*buf = append(*buf, uint8(_FIN|_CONTINUE))
	} else if !firstpiece {
		*buf = append(*buf, uint8(_CONTINUE))
	} else {
		*buf = append(*buf, msgtype)
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
		*buf = append(*buf, _MASK|payload)
	} else {
		*buf = append(*buf, payload)
	}
	switch payload {
	case 127:
		*buf = (*buf)[:10]
		binary.BigEndian.PutUint64((*buf)[2:], uint64(length))
	case 126:
		*buf = (*buf)[:4]
		binary.BigEndian.PutUint16((*buf)[2:], uint16(length))
	}
	if mask {
		*buf = (*buf)[:len(*buf)+4]
		rand.Read((*buf)[len(*buf)-4:])
		maskkey = (*buf)[len(*buf)-4:]
	}
	return
}

// RFC 6455: all message from client to server must be masked
func WriteMsg(conn net.Conn, data []byte, fin, firstpiece, mask bool) error {
	buf := bpool.Get(len(data) + 14)
	defer bpool.Put(&buf)
	maskkey := makeheader(&buf, fin, firstpiece, mask, uint64(len(data)), uint8(_BINARY))
	headlen := len(buf)
	buf = append(buf, data...)
	if mask {
		domask(buf[headlen:], maskkey)
	}
	if _, e := conn.Write(buf); e != nil {
		return e
	}
	return nil
}

// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
// RFC 6455: all message from client to server must be masked
func WritePing(conn net.Conn, data []byte, mask bool) error {
	if len(data) > 125 {
		return ErrMsgLarge
	}
	buf := bpool.Get(6 + len(data))
	defer bpool.Put(&buf)
	maskkey := makeheader(&buf, true, true, mask, uint64(len(data)), uint8(_PING))
	headlen := len(buf)
	buf = append(buf, data...)
	if mask {
		domask(buf[headlen:], maskkey)
	}
	if _, e := conn.Write(buf); e != nil {
		return e
	}
	return nil
}

// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
// RFC 6455: all message from client to server must be masked
func WritePong(conn net.Conn, data []byte, mask bool) error {
	if len(data) > 125 {
		return ErrMsgLarge
	}
	buf := bpool.Get(6 + len(data))
	defer bpool.Put(&buf)
	maskkey := makeheader(&buf, true, true, mask, uint64(len(data)), uint8(_PONG))
	headlen := len(buf)
	buf = append(buf, data...)
	if mask {
		domask(buf[headlen:], maskkey)
	}
	if _, e := conn.Write(buf); e != nil {
		return e
	}
	return nil
}
