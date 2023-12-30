package ws

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"

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

func decodeFirstSecond(reader *bufio.Reader) (fin, rsv1, rsv2, rsv3 bool, opcode OPCode, mask bool, payloadlen uint32, e error) {
	var b byte
	b, e = reader.ReadByte()
	if e != nil {
		return
	}
	if b&_FIN > 0 {
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
	opcode = OPCode(b & 0b00001111)
	if opcode != _CONTINUE && opcode != _TEXT && opcode != _BINARY && opcode != _CLOSE && opcode != _PING && opcode != _PONG {
		e = ErrMsgType
		return
	}
	if opcode.IsControl() && !fin {
		// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
		e = ErrMsgFin
		return
	}
	b, e = reader.ReadByte()
	if b&_MASK > 0 {
		mask = true
	}
	payloadlen = uint32(b & 0b01111111)
	if opcode.IsControl() && payloadlen > 125 {
		// RFC 6455: All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented.
		e = ErrMsgLarge
	}
	return
}

// RFC 6455: all message from client to server must be masked
func Read(reader *bufio.Reader, maxmsglen uint32, mustmask bool, handler func(OPCode, []byte) (readmore bool)) error {
	code := _CONTINUE
	var buf []byte
	defer func() {
		if buf != nil {
			pool.GetPool().Put(&buf)
		}
	}()
	for {
		fin, _, _, _, curcode, mask, payloadlen, e := decodeFirstSecond(reader)
		if e != nil {
			return e
		}
		if mustmask && !mask {
			return ErrMsgMask
		}
		if !curcode.IsControl() {
			if code == _CONTINUE {
				if curcode == _CONTINUE {
					return ErrMsgType
				}
				code = curcode
			} else if code != curcode && curcode != _CONTINUE {
				return ErrMsgType
			}
		}
		if buf == nil {
			buf = pool.GetPool().Get(256)
			buf = buf[:8]
		}
		switch payloadlen {
		case 127:
			if _, e := io.ReadFull(reader, buf[:8]); e != nil {
				return e
			}
			tmplen := binary.BigEndian.Uint64(buf[:8])
			if tmplen > math.MaxUint32 {
				return ErrMsgLarge
			}
			payloadlen = uint32(tmplen)
		case 126:
			if _, e := io.ReadFull(reader, buf[:2]); e != nil {
				return e
			}
			payloadlen = uint32(binary.BigEndian.Uint16(buf[:2]))
		}
		if payloadlen > maxmsglen || (!curcode.IsControl() && len(buf) > 8 && len(buf)-8+int(payloadlen) > int(maxmsglen)) {
			return ErrMsgLarge
		}
		if mask {
			if _, e := io.ReadFull(reader, buf[:4]); e != nil {
				return e
			}
		}
		if payloadlen > 0 {
			buf = pool.CheckCap(&buf, len(buf)+int(payloadlen))
			buf = buf[:len(buf)+int(payloadlen)]
			if _, e := io.ReadFull(reader, buf[len(buf)-int(payloadlen):]); e != nil {
				return e
			}
		}
		if mask && payloadlen > 0 {
			domask(buf[len(buf)-int(payloadlen):], buf[:4])
		}
		if curcode.IsControl() {
			if !handler(curcode, buf[len(buf)-int(payloadlen):]) {
				return nil
			}
			buf = buf[:len(buf)-int(payloadlen)]
		} else if fin {
			if !handler(code, buf[8:]) {
				return nil
			}
			if len(buf) > 4096 {
				pool.GetPool().Put(&buf)
				buf = nil
			} else {
				buf = buf[:8]
			}
			code = _CONTINUE
		}
	}
}
