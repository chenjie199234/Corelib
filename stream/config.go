package stream

type Config struct {
	//the name of this instance
	SelfName string
	//two peers need to verify each other,before they can communicate
	VerifyTimeout int64 //default 1000ms

	//heartbeat timeout
	HeartTimeout int64 //default 5000ms

	//how many samples a cycle
	NetLagSampleNum int

	//split connections into pieces
	//every piece will have an independence RWMutex to control online and offline
	//every piece will have an independence goruntine to check heart timeout nodes in this piece
	Splitnum int //default 1

	//for Tcp server's and client's socket buffer
	TcpSocketReadBufferLen  int //default 1024
	TcpSocketWriteBufferLen int //default 1024

	//for Unix server's and client's socket buffer
	UnixSocketReadBufferLen  int //default 4096
	UnixSocketWriteBufferLen int //default 4096

	//read buffer can auto grow and shirnk within min and max
	AppMinReadBufferLen int //the num of byte,default 1024
	AppMaxReadBufferLen int //the num of byte,default 65535
	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int //the num of message packages,default 256

	//for websocket server and client
	WebSocketReadheaderTimeout int //default 250ms
	WebSocketHandshakeTimeout  int //default 500ms

	//websocket's tcp server's and client's socket buffer
	WebSocketMaxHeader      int //default 1024
	WebSocketReadBufferLen  int //default 1024
	WebSocketWriteBufferLen int //default 1024
	WebSocketEnableCompress bool
	TlsCertFile             string
	TlsKeyFile              string
}

func checkConfig(c *Config) {
	//tcp socket config
	if c.TcpSocketReadBufferLen == 0 {
		c.TcpSocketReadBufferLen = 1024
	}
	if c.TcpSocketWriteBufferLen == 0 {
		c.TcpSocketWriteBufferLen = 1024
	}
	//unix socket config
	if c.UnixSocketReadBufferLen == 0 {
		c.TcpSocketReadBufferLen = 4096
	}
	if c.UnixSocketWriteBufferLen == 0 {
		c.TcpSocketWriteBufferLen = 4096
	}
	//app buffer config
	if c.AppWriteBufferNum == 0 {
		c.AppWriteBufferNum = 256
	}
	if c.AppMinReadBufferLen == 0 {
		c.AppMinReadBufferLen = 1024
	}
	if c.AppMaxReadBufferLen == 0 {
		c.AppMaxReadBufferLen = 40960
	}
	if c.AppMaxReadBufferLen < c.AppMinReadBufferLen {
		c.AppMaxReadBufferLen = c.AppMinReadBufferLen
	}
	//web socket config
	if c.WebSocketReadheaderTimeout == 0 {
		c.WebSocketReadheaderTimeout = 250
	}
	if c.WebSocketHandshakeTimeout == 0 {
		c.WebSocketHandshakeTimeout = 500
	}
	if c.WebSocketReadBufferLen == 0 {
		c.WebSocketReadBufferLen = 1024
	}
	if c.WebSocketWriteBufferLen == 0 {
		c.WebSocketWriteBufferLen = 1024
	}
	if c.WebSocketMaxHeader == 0 {
		c.WebSocketMaxHeader = 1024
	}
	//global config
	if c.VerifyTimeout == 0 {
		c.VerifyTimeout = 1000
	}
	if c.HeartTimeout == 0 {
		c.HeartTimeout = 5000
	}
	if c.NetLagSampleNum == 0 {
		c.NetLagSampleNum = 10
	}
	if c.Splitnum == 0 {
		c.Splitnum = 1
	}
}
