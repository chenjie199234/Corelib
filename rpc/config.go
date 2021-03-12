package rpc

import (
	"time"
)

type Config struct {
	Timeout                time.Duration //global timeout for every rpc call
	ConnTimeout            time.Duration
	HeartTimeout           time.Duration
	HeartPorbe             time.Duration
	GroupNum               uint
	SocketRBuf             uint
	SocketWBuf             uint
	MaxMsgLen              uint
	MaxBufferedWriteMsgNum uint
}
