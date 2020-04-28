package stream

type Config struct {
	//two peers need to verify each other,before they can communicate
	VerifyTimeout int64
	HeartInterval int64
	//read buffer can auto grow and shirnk within min and max
	MinReadBufferLen int //the num of byte,default 1024
	MaxReadBufferLen int //the num of byte,default 65535
	//write buffer can store the messages in buffer and send async in another goruntine
	MaxWriteBufferNum int //the num of message packages,default 256
	//split connections into pieces
	//every piece will have an independence RWMutex to control online and offline
	//every piece will have an independence goruntine to check heart timeout
	Splitnum int //default 1
}
