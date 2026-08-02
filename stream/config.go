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
// if uniqueid is empty,the peer's RemoteAddr(ip:port) will be used as the uniqueid
// Warning!!!Don't reuse the data in 'peerVerifyData',it will change when this function return,if you want to use it,copy it first
type HandleVerifyFunc func(ctx context.Context, peerVerifyData []byte) (response []byte, uniqueid string, success bool)

// This is a notice func after verify each other success
// success = true means online success
// success = false means online failed,connection will be closed
type HandleOnlineFunc func(ctx context.Context, p *Peer) (success bool)

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

type InstanceConfig struct {
	//3 probe missing means disconnect,this is used to detect the connection's alive status
	//min 1s,default 5s
	HeartprobeInterval time.Duration
	//time for connection establish(include dial time,tls handshake time and verify time)
	//default 3s
	ConnectTimeout time.Duration
	//time for read one complete message,start from read the first byte of the message
	//this can be used to prevent client's slow send attack
	//(send a big message,but every time send 1 byte,the connection is alive and works well,attacker can create lots of this kind of connections)
	//<=0 means no timeout
	ReadTimeout time.Duration
	//time for write one piece of the complete message(big message will split to many pieces to send,one piece's max len is 65536)
	//the connection's write deadline will be resetted every time start to write a piece of data
	//this can be used to prevent client's slow read attack
	//(attacker set tcp a small window,and read slowly,this will block the server's Send action,attacker can create lots of this kind of connections)
	//<=0 means no timeout
	WriteTimeout time.Duration
	//time for waiting the next user message,it will be reset when starting to read next message
	//expired without next message,the connection will be closed
	//<=0 means no timeout
	IdleTimeout time.Duration

	//split connections into groups
	//each group has an independence RWMutex to control online and offline
	//each group's connections' heart probe check is in an independence goroutine
	//small group num will increase to lock conflict
	//big group num will increate the goroutine num
	//default 100,max 50000
	GroupNum uint16

	//min 64k,default 64M
	MaxMsgLen uint32

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
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 3 * time.Second
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
	if c.GroupNum == 0 {
		c.GroupNum = 100
	} else if c.GroupNum > 50000 {
		c.GroupNum = 50000
	}
}

func (c *InstanceConfig) clone() *InstanceConfig {
	return &InstanceConfig{
		HeartprobeInterval: c.HeartprobeInterval,
		ConnectTimeout:     c.ConnectTimeout,
		ReadTimeout:        c.ReadTimeout,
		WriteTimeout:       c.WriteTimeout,
		IdleTimeout:        c.IdleTimeout,
		GroupNum:           c.GroupNum,
		MaxMsgLen:          c.MaxMsgLen,
		VerifyFunc:         c.VerifyFunc,
		OnlineFunc:         c.OnlineFunc,
		PingPongFunc:       c.PingPongFunc,
		UserdataFunc:       c.UserdataFunc,
		OfflineFunc:        c.OfflineFunc,
	}
}
