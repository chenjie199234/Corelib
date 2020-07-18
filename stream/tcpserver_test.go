package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var tcpserverinstance *Instance
var tcpcount int64

func Test_Tcpserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	tcpserverinstance = NewInstance(&InstanceConfig{
		SelfName:           "server",
		VerifyTimeout:      500,
		VerifyData:         []byte{'t', 'e', 's', 't'},
		HeartbeatTimeout:   1500,
		HeartprobeInterval: 500,
		NetLagSampleNum:    10,
		GroupNum:           10,
		Verifyfunc:         tcpserverhandleVerify,
		Onlinefunc:         tcpserverhandleonline,
		Userdatafunc:       tcpserverhandleuserdata,
		Offlinefunc:        tcpserverhandleoffline,
	})
	tcpserverinstance.StartTcpServer(&TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, "127.0.0.1:9234")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", tcpcount)
		}
	}()
	http.ListenAndServe(":8080", nil)
}
func tcpserverhandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}
func tcpserverhandleonline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&tcpcount, 1)
}
func tcpserverhandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s:%s\n", peername, data)
	p.SendMessage(data, uniqueid)
}
func tcpserverhandleoffline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&tcpcount, -1)
}