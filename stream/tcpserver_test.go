package stream

import (
	"bytes"
	"context"
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
		HeartbeatTimeout:   1500,
		HeartprobeInterval: 500,
		RecvIdleTimeout:    30000, //30s
		GroupNum:           10,
		Verifyfunc:         tcpserverhandleVerify,
		Onlinefunc:         tcpserverhandleonline,
		Userdatafunc:       tcpserverhandleuserdata,
		Offlinefunc:        tcpserverhandleoffline,
	})
	go tcpserverinstance.StartTcpServer(&TcpConfig{
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
func tcpserverhandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return []byte{'t', 'e', 's', 't'}, true
}
func tcpserverhandleonline(p *Peer, peeruniquename string, starttime uint64) {
	atomic.AddInt64(&tcpcount, 1)
}
func tcpserverhandleuserdata(p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s:%s\n", peeruniquename, data)
	p.SendMessage(data, starttime)
}
func tcpserverhandleoffline(p *Peer, peeruniquename string, starttime uint64) {
	atomic.AddInt64(&tcpcount, -1)
}
