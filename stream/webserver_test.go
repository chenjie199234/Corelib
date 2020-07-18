package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var webserverinstance *Instance
var webcount int64

func Test_Webserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	webserverinstance = NewInstance(&InstanceConfig{
		SelfName:           "server",
		VerifyTimeout:      500,
		VerifyData:         []byte{'t', 'e', 's', 't'},
		HeartbeatTimeout:   1500,
		HeartprobeInterval: 500,
		NetLagSampleNum:    10,
		GroupNum:           10,
		Verifyfunc:         webserverhandleVerify,
		Onlinefunc:         webserverhandleonline,
		Userdatafunc:       webserverhandleuserdata,
		Offlinefunc:        webserverhandleoffline,
	})
	os.Remove("./test.socket")
	webserverinstance.StartWebsocketServer(&WebConfig{
		ConnectTimeout:       1000,
		HttpMaxHeaderLen:     1024,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppWriteBufferNum:    256,
	}, []string{"/test"}, "127.0.0.1:9234", func(*http.Request) bool { return true })
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", webcount)
		}
	}()
	http.ListenAndServe(":8080", nil)
}
func webserverhandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}
func webserverhandleonline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&webcount, 1)
}
func webserverhandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s:%s\n", peername, data)
	p.SendMessage(data, uniqueid)
}
func webserverhandleoffline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&webcount, -1)
}
