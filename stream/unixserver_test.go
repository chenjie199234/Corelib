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

var unixserverinstance *Instance
var unixcount int64

func Test_Unixserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	unixserverinstance = NewInstance(&Config{
		SelfName:        "server",
		VerifyTimeout:   500,
		HeartTimeout:    1000,
		NetLagSampleNum: 10,
		Splitnum:        10,
	}, unixserverhandleVerify, unixserverhandleonline, unixserverhandleuserdata, unixserverhandleoffline)
	os.Remove("./test.socket")
	unixserverinstance.StartUnixsocketServer([]byte{}, "./test.socket")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", unixcount)
		}
	}()
	http.ListenAndServe(":8080", nil)
}
func unixserverhandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}
func unixserverhandleonline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&unixcount, 1)
}
func unixserverhandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s:%s\n", peername, data)
	p.SendMessage(data, uniqueid)
}
func unixserverhandleoffline(p *Peer, peername string, uniqueid int64) {
	atomic.AddInt64(&unixcount, -1)
}
