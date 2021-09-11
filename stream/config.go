package stream

import (
	"context"
	"time"
)

//Warning!!Don't write block logic in these callback,live for{}

//peeruniquename = peername:peeraddr,e.g. "gamegate:127.0.0.1:1234"

//HandleVerifyFunc has a timeout context
//Before two peers can communicate with each other,they need to verify the identity first
//server's response will write back to the client for client to verify the server
//client's response is useless and it will be dropped,you can just return nil
//Return true means verify success in business logic
//Return false means verify failed in business logic
//Don't reuse peerVerifyData in this function,the under layer data in 'peerVerifyData' will change when this function return
type HandleVerifyFunc func(ctx context.Context, peeruniquename string, peerVerifyData []byte) (response []byte, success bool)

//This is a notice after verify success
//Return true means online success in business logic
//Return false means online failed in business logic
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandleOnlineFunc func(p *Peer, peeruniquename string, sid int64) (success bool)

//This is a func to deal the user message
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
//Don't reuse 'data' in this function,the under layer data in 'data' will change when this function return
type HandleUserdataFunc func(p *Peer, peeruniquename string, data []byte, sid int64)

//This is a notice before two peers disconnect with each other
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
//After this notice the peer is unknown,dont't use it anymore
type HandleOfflineFunc func(p *Peer, peeruniquename string)

type TcpConfig struct {
	//include connect time and verify time
	ConnectTimeout time.Duration //default 500ms

	SocketRBufLen uint32 //default 1024 byte,max 65535 byte
	SocketWBufLen uint32 //default 1024 byte,max 65535 byte

	MaxMsgLen uint32 //min 1024,max 65535,default is max
}

var defaultTcpConfig = &TcpConfig{
	ConnectTimeout: 500 * time.Millisecond,
	SocketRBufLen:  1024,
	SocketWBufLen:  1024,
	MaxMsgLen:      65535,
}

func (c *TcpConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
	}
	if c.SocketRBufLen == 0 {
		c.SocketRBufLen = 1024
	}
	if c.SocketRBufLen > 65535 {
		c.SocketRBufLen = 65535
	}
	if c.SocketWBufLen == 0 {
		c.SocketWBufLen = 1024
	}
	if c.SocketWBufLen > 65535 {
		c.SocketWBufLen = 65535
	}
	if c.MaxMsgLen < 1024 {
		c.MaxMsgLen = 65535
	}
	if c.MaxMsgLen > 65535 {
		c.MaxMsgLen = 65535
	}
}

type TcpClientOnlyConfig struct {
}

var defaultTcpClientOnlyConfig = &TcpClientOnlyConfig{}

func (c *TcpClientOnlyConfig) validate() {
}

type TcpServerOnlyConfig struct {
}

var defaultTcpServerOnlyConfig = &TcpServerOnlyConfig{}

func (c *TcpServerOnlyConfig) validate() {

}

type InstanceConfig struct {
	//heartbeat timeout
	HeartbeatTimeout time.Duration //default 5s
	//must < HeartbeatTimeout
	HeartprobeInterval time.Duration //default 1.5s
	//only reveice heartbeat msg and no more userdata msg is idle
	//0 means no idle timeout
	//every userdata msg will recycle the timeout
	RecvIdleTimeout time.Duration
	//if this is not 0,this must > HeartprobeInterval,this is useful for slow read attact
	SendIdleTimeout time.Duration //default HeartprobeInterval + (1 second)
	//write buffer can store the messages in buffer and send async in another goruntine
	MaxBufferedWriteMsgNum uint32 //default 256 num(not the byte)

	//split connections into groups
	//every group will have an independence RWMutex to control online and offline
	//every group will have an independence goruntine to check nodes' heart timeout in this group
	GroupNum uint32 //default 1 num

	//specify the tcp socket connection's config
	TcpC       *TcpConfig
	TcpClientC *TcpClientOnlyConfig
	TcpServerC *TcpServerOnlyConfig

	//before peer and peer confirm connection,they need to verify each other
	//after tcp connected,this function will be called
	Verifyfunc HandleVerifyFunc
	//this function will be called after peer and peer verified each other
	Onlinefunc HandleOnlineFunc
	//this function used to deal userdata
	Userdatafunc HandleUserdataFunc
	//this function will be called when peer and peer closed their connection
	Offlinefunc HandleOfflineFunc
}

func (c *InstanceConfig) validate() {
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = 5 * time.Second
	}
	if c.HeartprobeInterval <= 0 {
		c.HeartprobeInterval = 1500 * time.Millisecond
	}
	if c.HeartprobeInterval >= c.HeartbeatTimeout {
		c.HeartprobeInterval = c.HeartbeatTimeout / 3
	}
	if c.RecvIdleTimeout < 0 {
		c.RecvIdleTimeout = 0
	}
	if c.SendIdleTimeout <= c.HeartprobeInterval {
		c.SendIdleTimeout = c.HeartprobeInterval + time.Second
	}
	if c.MaxBufferedWriteMsgNum == 0 {
		c.MaxBufferedWriteMsgNum = 256
	}
	if c.GroupNum == 0 {
		c.GroupNum = 1
	}
	if c.TcpC == nil {
		c.TcpC = defaultTcpConfig
	} else {
		c.TcpC.validate()
	}
	if c.TcpClientC == nil {
		c.TcpClientC = defaultTcpClientOnlyConfig
	} else {
		c.TcpClientC.validate()
	}
	if c.TcpServerC == nil {
		c.TcpServerC = defaultTcpServerOnlyConfig
	} else {
		c.TcpServerC.validate()
	}
}
