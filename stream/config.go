package stream

type Config struct {
	//two peers need to verify each other,before they can communicate
	VerifyTimeout int64 //default 1000ms

	//heartbeat timeout
	HeartInterval int64 //default 5000ms

	//how many samples a cycle
	NetLagSampleNum int

	//split connections into pieces
	//every piece will have an independence RWMutex to control online and offline
	//every piece will have an independence goruntine to check heart timeout nodes in this piece
	Splitnum int //default 1

	//for Tcp and Websocket server and client
	TcpSocketReadBufferLen  int //default 1024
	TcpSocketWriteBufferLen int //default 1024

	//for UnixSocket server and client
	UnixSocketReadBufferLen  int //default 4096
	UnixSocketWriteBufferLen int //default 4096

	//app read buffer can auto grow and shirnk within min and max
	AppMinReadBufferLen int //the num of byte,default 1024
	AppMaxReadBufferLen int //the num of byte,default 65535
	//write buffer can store the messages in buffer and send async in another goruntine
	AppWriteBufferNum int //the num of message packages,default 256
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
	//global config
	if c.VerifyTimeout == 0 {
		c.VerifyTimeout = 1000
	}
	if c.HeartInterval == 0 {
		c.HeartInterval = 5000
	}
	if c.NetLagSampleNum == 0 {
		c.NetLagSampleNum = 10
	}
	if c.Splitnum == 0 {
		c.Splitnum = 1
	}
}
