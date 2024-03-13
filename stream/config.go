package stream

import (
	"context"
	"time"
)

//Warning!!Don't write block logic in these callback,live for{}

//peeruniquename = peername:peeraddr,e.g. "gamegate:127.0.0.1:1234"

// HandleVerifyFunc has a timeout context,timeout depend on the ConnectTimeout in config
// Before two peers can communicate,they need to verify each other first
// server's response will write back to the client for client to verify the server
// client's response is useless and it will be dropped,you can just return nil
// if uniquekey is empty,the peer's RemoteAddr(ip:port) will be used as the uniquekey
// Warning!!!Don't reuse the data in 'peerVerifyData',it will change when this function return,if you want to use it,copy it first
type HandleVerifyFunc func(ctx context.Context, peerVerifyData []byte) (response []byte, uniquekey string, success bool)

// This is a notice func after verify each other success
// success = true means online success
// success = false means online failed,connection will be closed
// Peer is a cancel context,it will be canceled when the connection closed
// You can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandleOnlineFunc func(p *Peer) (success bool)

// This is a notice func about which peer is alive
// Peer is a cancel context,it will be canceled when the connection closed
// You can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandlePingPongFunc func(p *Peer)

// This is a func to deal the user message
// Peer is a cancel context,it will be canceled when the connection closed
// You can control the timeout by yourself through context.WithTimeout(p,time.Second)
// Warning!!!Don't reuse the data in 'userdata',it will change when this function return,if you want to use it,copy it first
type HandleUserdataFunc func(p *Peer, userdata []byte)

// This is a notice func after two peers disconnect with each other
// Peer is a cancel context,it will be canceled after this function return
// You can control the timeout by yourself through context.WithTimeout(p,time.Second)
// After this notice the peer is unknown,dont't use it anymore
type HandleOfflineFunc func(p *Peer)

type TcpConfig struct {
	//time for connection establish(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout time.Duration
	//min 64k,default 64M
	MaxMsgLen uint32
}

var defaultTcpConfig = &TcpConfig{
	ConnectTimeout: time.Second * 3,
	MaxMsgLen:      1024 * 1024 * 64,
}

func (c *TcpConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 3 * time.Second
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
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
	//3 probe missing means disconnect
	//min 1s,default 5s
	HeartprobeInterval time.Duration
	//recv idle means no userdata msg received
	//every userdata msg will refresh the timeout(only userdata msg)
	//<=0 means no idle timeout,if >0 min is HeartprobeInterval
	RecvIdleTimeout time.Duration
	//every msg's send will refresh the timeout(all kind of msg)
	//min HeartprobeInterval * 3,default HeartprobeInterval * 3
	SendIdleTimeout time.Duration

	//split connections into groups
	//every group will have an independence RWMutex to control online and offline
	//default 100
	GroupNum uint16

	//specify the tcp socket connection's config
	TcpC       *TcpConfig
	TcpClientC *TcpClientOnlyConfig
	TcpServerC *TcpServerOnlyConfig

	//before peer and peer confirm connection,they need to verify each other
	//after tcp connected,this function will be called
	VerifyFunc HandleVerifyFunc
	//this function will be called after peer and peer verified each other
	OnlineFunc HandleOnlineFunc
	//this function used to tel user which peer is alive
	PingPongFunc HandlePingPongFunc
	//this function used to deal userdata
	UserdataFunc HandleUserdataFunc
	//this function will be called when peer and peer closed their connection
	OfflineFunc HandleOfflineFunc
}

func (c *InstanceConfig) validate() {
	if c.HeartprobeInterval <= 0 {
		c.HeartprobeInterval = time.Second * 5
	} else if c.HeartprobeInterval < time.Second {
		c.HeartprobeInterval = time.Second
	}
	if c.RecvIdleTimeout < 0 {
		c.RecvIdleTimeout = 0
	} else if c.RecvIdleTimeout > 0 && c.RecvIdleTimeout < c.HeartprobeInterval {
		c.RecvIdleTimeout = c.HeartprobeInterval
	}
	if c.SendIdleTimeout < c.HeartprobeInterval*3 {
		c.SendIdleTimeout = c.HeartprobeInterval * 3
	}
	if c.GroupNum == 0 {
		c.GroupNum = 100
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
