package stream

import (
	"context"
	"fmt"
)

//Warning!!Don't write block logic in these callback,live for{}

//peeruniquename = peername:peeraddr,e.g. "gamegate:127.0.0.1:1234"

//HandleVerifyFunc has a timeout context
//Before two peers can communicate with each other,they need to verify the identity first
//server's response will write back to the client for client to verify the server
//client's response is useless and it will be dropped,you can just return nil
type HandleVerifyFunc func(ctx context.Context, peeruniquename string, peerVerifyData []byte) (response []byte, success bool)

//This is a notice after two peers verify each other success
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandleOnlineFunc func(p *Peer, peeruniquename string, starttime uint64)

//This is a func to deal the user message
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
type HandleUserdataFunc func(p *Peer, peeruniquename string, data []byte, starttime uint64)

//This is a notice before two peers disconnect with each other
//Peer is a cancel context,it will be canceled when the connection closed,and you can control the timeout by yourself through context.WithTimeout(p,time.Second)
//After this notice the peer is unknown,dont't use it anymore
type HandleOfflineFunc func(p *Peer, peeruniquename string, starttime uint64)

type TcpConfig struct {
	ConnectTimeout int `json:"connect_timeout"` //default 500ms,for client only

	SocketReadBufferLen  int `json:"socket_read_buffer_len"`  //default 1024 byte,max 65535 byte
	SocketWriteBufferLen int `json:"socket_write_buffer_len"` //default 1024 byte,max 65535 byte

	//read buffer can auto grow and shirnk within min and max
	AppMinReadBufferLen int `json:"app_min_read_buffer_len"`  //default 1024 byte,max 65535 byte
	AppMaxReadBufferLen int `json:"app_max_write_buffer_len"` //default 65535 byte,max 65535 byte
	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int `json:"app_write_buffer_num"` //default 256 num(not the byte)
}

func checkTcpConfig(c *TcpConfig) {
	if c.ConnectTimeout == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing connect timeout,default will be used:500ms")
		c.ConnectTimeout = 500
	}
	if c.SocketReadBufferLen == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing socket read buffer len,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketReadBufferLen > 65535 {
		fmt.Println("[Stream.checkTcpConfig]socket read buffer len is too large,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketWriteBufferLen == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing socket write buffer len,default will be used:1024 byte")
		c.SocketWriteBufferLen = 1024
	}
	if c.SocketWriteBufferLen > 65535 {
		fmt.Println("[Stream.checkTcpConfig]socket write buffer len is too large,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.AppMinReadBufferLen == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing app min read buffer len,default will be used:1024 byte")
		c.AppMinReadBufferLen = 1024
	}
	if c.AppMinReadBufferLen > 65535 {
		fmt.Println("[Stream.checkTcpConfig]app min read buffer len is too large,default will be used:1024 byte")
		c.AppMinReadBufferLen = 1024
	}
	if c.AppMaxReadBufferLen == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing app max read buffer len,default will be used:65535 byte")
		c.AppMaxReadBufferLen = 65535
	}
	if c.AppMaxReadBufferLen > 65535 {
		fmt.Println("[Stream.checkTcpConfig]app max read buffer len is too large,default will be used:65535 byte")
		c.AppMaxReadBufferLen = 65535
	}
	if c.AppMinReadBufferLen > c.AppMaxReadBufferLen {
		fmt.Println("[Stream.checkTcpConfig]app 'min read buffer len' > 'max read buffer len','max read buffer len' will be set to 'min read buffer len'")
		c.AppMaxReadBufferLen = c.AppMinReadBufferLen
	}
	if c.AppWriteBufferNum == 0 {
		fmt.Println("[Stream.checkTcpConfig]missing app write buffer num,default will be used:256 num")
		c.AppWriteBufferNum = 256
	}
}

type UnixConfig struct {
	ConnectTimeout int `json:"connect_timeout"` //default 500ms,for client only

	SocketReadBufferLen  int `json:"socket_read_buffer_len"`  //default 1024 byte,max 65535 byte
	SocketWriteBufferLen int `json:"socket_write_buffer_len"` //default 1024 byte,max 65535 byte

	//read buffer can auto grow and shirnk within min and max
	AppMinReadBufferLen int `json:"app_min_read_buffer_len"`  //default 1024 byte,max 65535 byte
	AppMaxReadBufferLen int `json:"app_max_write_buffer_len"` //default 65535 byte,max 65535 byte
	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int `json:"app_write_buffer_num"` //default 256 num(not the byte)
}

func checkUnixConfig(c *UnixConfig) {
	if c.ConnectTimeout == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing connect timeout,default will be used:500ms")
		c.ConnectTimeout = 500
	}
	if c.SocketReadBufferLen == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing socket read buffer len,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketReadBufferLen > 65535 {
		fmt.Println("[Stream.checkUnixConfig]socket read buffer len is too large,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketWriteBufferLen == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing socket write buffer len,default will be used:1024 byte")
		c.SocketWriteBufferLen = 1024
	}
	if c.SocketWriteBufferLen > 65535 {
		fmt.Println("[Stream.checkUnixConfig]socket write buffer len is too large,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.AppMinReadBufferLen == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing app min read buffer len,default will be used:1024 byte")
		c.AppMinReadBufferLen = 1024
	}
	if c.AppMinReadBufferLen > 65535 {
		fmt.Println("[Stream.checkUnixConfig]app min read buffer len is too large,default will be used:1024 byte")
		c.AppMinReadBufferLen = 1024
	}
	if c.AppMaxReadBufferLen == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing app max read buffer len,default will be used:65535 byte")
		c.AppMaxReadBufferLen = 65535
	}
	if c.AppMaxReadBufferLen > 65535 {
		fmt.Println("[Stream.checkUnixConfig]app max read buffer len is too large,default will be used:65535 byte")
		c.AppMaxReadBufferLen = 65535
	}
	if c.AppMinReadBufferLen > c.AppMaxReadBufferLen {
		fmt.Println("[Stream.checkUnixConfig]app 'min read buffer len' > 'max read buffer len','max read buffer len' will be set to 'min read buffer len'")
		c.AppMaxReadBufferLen = c.AppMinReadBufferLen
	}
	if c.AppWriteBufferNum == 0 {
		fmt.Println("[Stream.checkUnixConfig]missing app write buffer num,default will be used:256 num")
		c.AppWriteBufferNum = 256
	}
}

type WebConfig struct {
	//for client this is the time to build connection with server
	//for server this is the time to upgrade connection to websocket
	ConnectTimeout       int `json:"connect_timeout"`         //default 500ms
	HttpMaxHeaderLen     int `json:"http_max_header_len"`     //default 1024 byte
	SocketReadBufferLen  int `json:"socket_read_buffer_len"`  //default 1024 byte
	SocketWriteBufferLen int `json:"socket_write_buffer_len"` //default 1024 byte
	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int    `json:"app_write_buffer_num"` //default 256 num(not the byte)
	EnableCompress    bool   `json:"enable_compress"`      //default false
	TlsCertFile       string `json:"tls_cert_file"`        //default don't use tls
	TlsKeyFile        string `json:"tls_key_file"`         //default don't use tls
}

func checkWebConfig(c *WebConfig) {
	if c.ConnectTimeout == 0 {
		fmt.Println("[Stream.checkWebConfig]missing connect timeout,default will be used:500ms")
		c.ConnectTimeout = 500
	}
	if c.HttpMaxHeaderLen == 0 {
		fmt.Println("[Stream.checkWebConfig]missing http max header len,default will be used:1024 byte")
		c.HttpMaxHeaderLen = 1024
	}
	if c.SocketReadBufferLen == 0 {
		fmt.Println("[Stream.checkWebConfig]missing socket read buffer len,default will be used:1024 byte")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketWriteBufferLen == 0 {
		fmt.Println("[Stream.checkWebConfig]missing socket write buffer len,default will be used:1024 byte")
		c.SocketWriteBufferLen = 1024
	}
	if c.AppWriteBufferNum == 0 {
		fmt.Println("[Stream.checkWebConfig]missing app write buffer num,default will be used:256 num")
		c.SocketWriteBufferLen = 256
	}
}

type InstanceConfig struct {
	//the name of this instance
	SelfName string `json:"self_name"`
	//two peers need to verify each other,before they can communicate
	VerifyTimeout uint64 `json:"verify_timeout"` //default 1000ms

	//heartbeat timeout
	HeartbeatTimeout   uint64 `json:"heartbeat_timeout"`   //default 5000ms
	HeartprobeInterval uint64 `json:"heartprobe_interval"` //default 1500ms
	//idle timeout,0 means no idle timeout
	IdleTimeout uint64 `json:"idle_timeout"`

	//split connections into groups
	//every group will have an independence RWMutex to control online and offline
	//every group will have an independence goruntine to check heart timeout nodes in this piece
	GroupNum uint `json:"group_num"` //default 1 num

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
	if c.SelfName == "" {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing instance name")
	}
	if len(c.SelfName) > 64 {
		return fmt.Errorf("[Stream.checkInstanceConfig]instance name too long")
	}
	for _, v := range c.SelfName {
		if (int(v) < 48 && int(v) != 46) || (int(v) > 57 && int(v) < 65) || (int(v) > 90 && int(v) < 97) || int(v) > 122 {
			return fmt.Errorf("[Stream.checkInstanceConfig]instance name contains illegal character which is not in [0-9],[a-z],[A-Z],[.]")
		}
	}
	if c.VerifyTimeout == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing verify timeout,default will be used:1000ms")
		c.VerifyTimeout = 1000
	}
	if c.HeartbeatTimeout == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing heartbeat timeout,default will be used:5000ms")
		c.HeartbeatTimeout = 5000
	}
	if c.HeartprobeInterval == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing heartprobe interval,default will be used:1500ms")
		c.HeartprobeInterval = 1500
	}
	if c.HeartprobeInterval >= c.HeartbeatTimeout {
		fmt.Println("[Stream.checkInstanceConfig]'heartbeat timeout' >= 'heartprobe interval','heartprobe interval' will be set to 'heartbeat timeout / 3'")
		c.HeartprobeInterval = c.HeartbeatTimeout / 3
	}
	if c.GroupNum == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing group num,default will be used:1 num")
		c.GroupNum = 1
	}
	//verify func can't be nill
	if c.Verifyfunc == nil {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing deal verify function")
	}
	//user data deal func can't be nill
	if c.Userdatafunc == nil {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing deal userdata function")
	}
	//online and offline func can be nill
	return nil
}
