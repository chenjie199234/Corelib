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

var serverinstance *Instance
var count int64

func Test_Tcpserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	serverinstance = NewInstance(&Config{
		VerifyTimeout: 250,
		HeartInterval: 500,
		Splitnum:      1024,
	}, serverhandleVerify, serverhandleonline, serverhandleuserdata, serverhandleoffline)
	serverinstance.StartTcpServer("server", []byte{}, "127.0.0.1:9234")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", count)
		}
	}()
	http.ListenAndServe(":8080", nil)
}
func serverhandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}
func serverhandleonline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&count, 1)
}
func serverhandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s:%s\n", peername, data)
	p.SendMessage(data, uniqueid)
}
func serverhandleoffline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&count, -1)
}
