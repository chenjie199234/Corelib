package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

var clientinstance *Instance

func Test_Tcpclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	clientinstance = NewInstance(&Config{
		VerifyTimeout: 250,
		HeartInterval: 500,
		Splitnum:      1024,
	}, clienthandleVerify, clienthandleonline, clienthandleuserdata, clienthandleoffline)
	go func() {
		for count := 0; count < 10000; count++ {
			clientinstance.StartTcpClient(fmt.Sprintf("client%d", count), []byte{}, "127.0.0.1:9234")
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func clienthandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func clienthandleonline(p *Peer, peername string, uniqueid int64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func clienthandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
}

func clienthandleoffline(p *Peer, peername string, uniqueid int64) {
}
