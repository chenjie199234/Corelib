package ws

import (
	"errors"
)

type OPCode uint8

const (
	_CONTINUE OPCode = 0b00000000 //can be not fin
	_TEXT     OPCode = 0b00000001 //can be not fin
	_BINARY   OPCode = 0b00000010 //can be not fin
	_CLOSE    OPCode = 0b00001000 //must be fin,payload <= 125
	_PING     OPCode = 0b00001001 //must be fin,payload <= 125
	_PONG     OPCode = 0b00001010 //must be fin,payload <= 125
	_RSV3            = 0b00010000
	_RSV2            = 0b00100000
	_RSV1            = 0b01000000
	_FIN             = 0b10000000
	_MASK            = 0b10000000
)

func (op OPCode) IsPing() bool {
	return op == _PING
}
func (op OPCode) IsPong() bool {
	return op == _PONG
}
func (op OPCode) IsClose() bool {
	return op == _CLOSE
}
func (op OPCode) IsControl() bool {
	return op&0b00001000 > 0
}

var ErrNotWS = errors.New("not a websocket connection")
var ErrRequestLineFormat = errors.New("http request line format wrong")
var ErrResponseLineFormat = errors.New("http response line format wrong")
var ErrHeaderLineFormat = errors.New("http header line format wrong")
var ErrAcceptSign = errors.New("accept sign wrong")

var ErrMsgLarge = errors.New("message too large")
var ErrMsgFin = errors.New("message must be fin")
var ErrMsgType = errors.New("message type unknown")
var ErrMsgMask = errors.New("message mask missing")

func domask(data []byte, maskkey []byte) {
	for i, v := range data {
		data[i] = v ^ maskkey[i%4]
	}
}
