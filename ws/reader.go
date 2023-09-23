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

// msg can be fragmented,so this can be happened:msg1 frame1,msg1 frame2,control frame,msg1 frame3,msg2 frame1
// when we get control frame,we need to deal this control frame,then continue to accept the rest msg
// example:
//
//	var conn net.Conn
//	... get the conn
//	reader := bufio.NewReader(conn)
//	msgbuf := pool.GetPool().Get(0)
//	defer pool.GetPool().Put(&msgbuf)
//	ctlbuf := pool.GetPool().Get(0)
//	defer pool.GetPool().Put(&ctlbuf)
//	for{
//		ctlcode, e :=Read(reader, &msgbuf, your_max_msg_length_limit, &ctlbuf)
//		if ctlcode.IsPing() {
//			//ping msg
//			ping := ctlbuf
//			... logic
//			ctlbuf = ctlbuf[:0]
//		}else if ctlcode.IsPong() {
//			//pong msg
//			pong := ctlbuf
//			... logic
//			ctlbuf = ctlbuf[:0]
//		}else if ctlcode.IsClose() {
//			//close msg
//			close := ctlbuf
//			... logic
//			ctlbuf = ctlbuf[:0]
//		}else{
//			//this is the msg
//			msg := msgbuf
//			... logic
//			msgbuf = msgbuf[:0]
//		}
//	}
//
// RFC 6455: all message from client to server must be masked
// ctlbuf's cap must >= 131
func Read(reader *bufio.Reader, msgbuf *[]byte, maxmsglen uint32, ctlbuf *[]byte, mustmask bool) (ctlcode OPCode, e error) {
	for {
		fin, _, _, _, opcode, mask, payloadlen, err := decodeFirstSecond(reader)
		if err != nil {
			return 0, err
		}
		if mustmask && !mask {
			return 0, ErrMsgMask
		}
		switch payloadlen {
		case 127:
			*ctlbuf = (*ctlbuf)[:8]
			if _, err := io.ReadFull(reader, *ctlbuf); err != nil {
				return 0, err
			}
			tmplen := binary.BigEndian.Uint64(*ctlbuf)
			if tmplen > math.MaxUint32 {
				return 0, ErrMsgLarge
			}
			payloadlen = uint32(tmplen)
			*ctlbuf = (*ctlbuf)[:0]
		case 126:
			*ctlbuf = (*ctlbuf)[:2]
			if _, err := io.ReadFull(reader, *ctlbuf); err != nil {
				return 0, err
			}
			payloadlen = uint32(binary.BigEndian.Uint16(*ctlbuf))
			*ctlbuf = (*ctlbuf)[:0]
		}
		if payloadlen > maxmsglen || (!opcode.IsControl() && uint64(len(*msgbuf))+uint64(payloadlen) > uint64(maxmsglen)) {
			return 0, ErrMsgLarge
		}
		if payloadlen == 0 {
			if mask {
				*ctlbuf = (*ctlbuf)[:4]
				if _, err := io.ReadFull(reader, *ctlbuf); err != nil {
					return 0, err
				}
				*ctlbuf = (*ctlbuf)[:0]
			}
			if fin {
				return opcode, nil
			}
			continue
		}
		if opcode.IsControl() {
			var maskkey []byte
			if mask {
				maskkey = pool.GetPool().Get(4)
				defer pool.GetPool().Put(&maskkey)
				if _, err := io.ReadFull(reader, maskkey); err != nil {
					return 0, err
				}
			}
			*ctlbuf = (*ctlbuf)[:payloadlen]
			if _, err := io.ReadFull(reader, *ctlbuf); err != nil {
				return 0, err
			}
			if mask {
				domask(*ctlbuf, maskkey)
			}
			return opcode, nil
		}
		if mask {
			*ctlbuf = (*ctlbuf)[:4]
			if _, err := io.ReadFull(reader, *ctlbuf); err != nil {
				return 0, err
			}
		}
		*msgbuf = pool.CheckCap(msgbuf, len(*msgbuf)+int(payloadlen))
		if _, e = io.ReadFull(reader, (*msgbuf)[uint32(len(*msgbuf))-payloadlen:]); e != nil {
			return
		}
		if mask {
			domask((*msgbuf)[uint32(len(*msgbuf))-payloadlen:], *ctlbuf)
		}
		*ctlbuf = (*ctlbuf)[:0]
		if fin {
			return _BINARY, nil
		}
	}
}
