package stream

import (
	"context"
	"fmt"
)

//Warning!!Don't write block logic in these callback,live for{}

//HandleVerifyFunc has a timeout context
type HandleVerifyFunc func(ctx context.Context, selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool

//HandleOnlineFunc has a cancel context,you should control the timeout by yourself through context.WithTimeout()
type HandleOnlineFunc func(ctx context.Context, p *Peer, peername string, uniqueid int64)

//HandleUserdataFunc has a cancel context,you should control the timeout by yourself through context.WithTimeout()
type HandleUserdataFunc func(ctx context.Context, p *Peer, peername string, uniqueid int64, data []byte)

//HandleOfflineFunc has a cancel context,you should control the timeout by yourself through context.WithTimeout()
type HandleOfflineFunc func(ctx context.Context, p *Peer, peername string, uniqueid int64)

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
	VerifyTimeout int64  `json:"verify_timeout"` //default 1000ms
	VerifyData    []byte `json:"verify_data"`

	//heartbeat timeout
	HeartbeatTimeout   int64 `json:"heartbeat_timeout"`   //default 5000ms
	HeartprobeInterval int64 `json:"heartprobe_interval"` //default 1500ms

	//how many samples a cycle
	NetLagSampleNum int `json:"netlag_sample_num"` //default 10 num

	//split connections into groups
	//every group will have an independence RWMutex to control online and offline
	//every group will have an independence goruntine to check heart timeout nodes in this piece
	GroupNum int `json:"group_num"` //default 1 num

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
	if c.VerifyTimeout == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing verify timeout,default will be used:1000ms")
		c.VerifyTimeout = 1000
	}
	if len(c.VerifyData) == 0 {
		return fmt.Errorf("[Stream.checkInstanceConfig]missing verify data")
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
	if c.NetLagSampleNum == 0 {
		fmt.Println("[Stream.checkInstanceConfig]missing netlag sample num,default will be used:10 num")
		c.NetLagSampleNum = 10
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
