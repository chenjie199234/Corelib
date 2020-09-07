package stream

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var unixserverinstance *Instance
var unixcount int64

func Test_Unixserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	unixserverinstance = NewInstance(&InstanceConfig{
		SelfName:           "server",
		VerifyTimeout:      500,
		HeartbeatTimeout:   1500,
		HeartprobeInterval: 500,
		NetLagSampleNum:    10,
		GroupNum:           10,
		Verifyfunc:         unixserverhandleVerify,
		Onlinefunc:         unixserverhandleonline,
		Userdatafunc:       unixclienthandleuserdata,
		Offlinefunc:        unixclienthandleoffline,
	})
	os.Remove("./test.socket")
	unixserverinstance.StartUnixsocketServer(&UnixConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, "./test.socket")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", unixcount)
		}
	}()
	http.ListenAndServe(":8082", nil)
}
func unixserverhandleVerify(ctx context.Context, peername string, peerVerifyData []byte) []byte {
	if len(peerVerifyData) != 0 {
		return nil
	}
	return []byte{'t', 'e', 's', 't'}
}
func unixserverhandleonline(p *Peer, peername string, uniqueid uint64) {
	atomic.AddInt64(&unixcount, 1)
}
func unixserverhandleuserdata(ctx context.Context, p *Peer, peername string, uniqueid uint64, data []byte) {
	fmt.Printf("%s:%s\n", peername, data)
	p.SendMessage(data, uniqueid)
}
func unixserverhandleoffline(p *Peer, peername string, uniqueid uint64) {
	atomic.AddInt64(&unixcount, -1)
}
