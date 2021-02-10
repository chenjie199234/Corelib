package stream

import (
	"context"
	"fmt"
	"time"

	"github.com/chenjie199234/Corelib/common"
)

//Warning!!Don't write block logic in these callback,live for{}

//peeruniquename = peername:peeraddr,e.g. "gamegate:127.0.0.1:1234"

//HandleVerifyFunc has a timeout context
//Before two peers can communicate with each other,they need to verify the identity first
//server's response will write back to the client for client to verify the server
//client's response is useless and it will be dropped,you can just return nil
//Don't reuse peerVerifyData in this function,the under layer data in 'peerVerifyData' will change when this function return
type HandleVerifyFunc func(ctx context.Context, peeruniquename string, peerVerifyData []byte) (response []byte, success bool)

//This is a notice after two peers verify each other success
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandleOnlineFunc func(p *Peer, peeruniquename string, starttime uint64)

//This is a func to deal the user message
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
//Don't reuse 'data' in this function,the under layer data in 'data' will change when this function return
type HandleUserdataFunc func(p *Peer, peeruniquename string, data []byte, starttime uint64)

//This is a notice before two peers disconnect with each other
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
//After this notice the peer is unknown,dont't use it anymore
type HandleOfflineFunc func(p *Peer, peeruniquename string, starttime uint64)

type TcpConfig struct {
	//include connect time and verify time
	ConnectTimeout time.Duration //default 500ms

	SocketReadBufferLen  int //default 1024 byte,max 65536 byte
	SocketWriteBufferLen int //default 1024 byte,max 65536 byte

	MaxMessageLen int //max 65536,default is max

	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int //default 256 num(not the byte)
}

var defaultTcpConfig = &TcpConfig{
	ConnectTimeout:       500 * time.Millisecond,
	SocketReadBufferLen:  1024,
	SocketWriteBufferLen: 1024,
	MaxMessageLen:        65536,
	AppWriteBufferNum:    256,
}

func (c *TcpConfig) checkTcpConfig() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
	}
	if c.SocketReadBufferLen <= 0 {
		c.SocketReadBufferLen = 1024
	}
	if c.SocketReadBufferLen > 65536 {
		c.SocketReadBufferLen = 65536
	}
	if c.SocketWriteBufferLen <= 0 {
		c.SocketWriteBufferLen = 1024
	}
	if c.SocketWriteBufferLen > 65536 {
		c.SocketReadBufferLen = 65536
	}
	if c.MaxMessageLen <= 0 {
		c.MaxMessageLen = 65536
	}
	if c.MaxMessageLen > 65536 {
		c.MaxMessageLen = 65536
	}
	if c.AppWriteBufferNum == 0 {
		c.AppWriteBufferNum = 256
	}
}

type UnixConfig struct {
	//include connect time and verify time
	ConnectTimeout time.Duration //default 500ms

	SocketReadBufferLen  int //default 1024 byte,max 65535 byte
	SocketWriteBufferLen int //default 1024 byte,max 65535 byte

	MaxMessageLen int //max 65536,default is max

	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int //default 256 num(not the byte)
}

var defaultUnixConfig = &UnixConfig{
	ConnectTimeout:       500 * time.Millisecond,
	SocketReadBufferLen:  1024,
	SocketWriteBufferLen: 1024,
	MaxMessageLen:        65536,
	AppWriteBufferNum:    256,
}

func (c *UnixConfig) checkUnixConfig() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 500 * time.Millisecond
	}
	if c.SocketReadBufferLen <= 0 {
		c.SocketReadBufferLen = 1024
	}
	if c.SocketReadBufferLen > 65536 {
		c.SocketReadBufferLen = 65536
	}
	if c.SocketWriteBufferLen <= 0 {
		c.SocketWriteBufferLen = 1024
	}
	if c.SocketWriteBufferLen > 65536 {
		c.SocketReadBufferLen = 65536
	}
	if c.MaxMessageLen <= 0 {
		c.MaxMessageLen = 65536
	}
	if c.MaxMessageLen > 65536 {
		c.MaxMessageLen = 65536
	}
	if c.AppWriteBufferNum == 0 {
		c.AppWriteBufferNum = 256
	}
}

type InstanceConfig struct {
	//the name of this instance
	SelfName string

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

	//split connections into groups
	//every group will have an independence RWMutex to control online and offline
	//every group will have an independence goruntine to check nodes' heart timeout in this group
	GroupNum uint //default 1 num

	//specify the tcp socket connection's config
	TcpC *TcpConfig
	//specify the unix socket connection's config
	UnixC *UnixConfig

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

func checkInstanceConfig(c *InstanceConfig) error {
	if e := common.NameCheck(c.SelfName, true); e != nil {
		return fmt.Errorf("[Stream.checkInstanceConfig] " + e.Error())
	}
	if c.HeartbeatTimeout == 0 {
		c.HeartbeatTimeout = 5 * time.Second
	}
	if c.HeartprobeInterval == 0 {
		c.HeartprobeInterval = 1500 * time.Millisecond
	}
	if c.HeartprobeInterval >= c.HeartbeatTimeout {
		c.HeartprobeInterval = c.HeartbeatTimeout / 3
	}
	if c.SendIdleTimeout <= c.HeartprobeInterval {
		c.SendIdleTimeout = c.HeartprobeInterval + time.Second
	}
	if c.GroupNum == 0 {
		c.GroupNum = 1
	}
	//verify func can't be nill
	//user data deal func can't be nill
	//online and offline func can be nill
	if c.Verifyfunc == nil {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing verify function")
	}
	if c.Userdatafunc == nil {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing userdata function")
	}
	if c.TcpC == nil {
		c.TcpC = defaultTcpConfig
	} else {
		c.TcpC.checkTcpConfig()
	}
	if c.UnixC == nil {
		c.UnixC = defaultUnixConfig
	} else {
		c.UnixC.checkUnixConfig()
	}
	return nil
}
