package logger

import (
	"net"
	"time"
	"unsafe"
)

type remote struct {
	remoteid  int64
	unixConn  *net.UnixConn
	tcpConn   *net.TCPConn
	notice    chan int
	lastHeart int64
	mq        *MQ
}

var DefaultLocalDir string = "./log"
var DefaultSplitSize int64 = 100 //unit M
var DefaultTimeout int64 = 1000  //unit millisecond
var DefaultNetLogNum uint32 = 512

var timeformat string = "2006-01-02-15_04_05-UTC"

func newLogfileName() string {
	return time.Now().UTC().Format(timeformat)
}

func level(lv int32) string {
	switch lv {
	case 1:
		return "Debug: "
	case 2:
		return "Info:  "
	case 3:
		return "Warn:  "
	case 4:
		return "Error: "
	case 5:
		return "Panic: "
	default:
		return ""
	}
}

func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
