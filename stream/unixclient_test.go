package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

var unixclientinstance *Instance

func Test_Unixclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	unixclientinstance = NewInstance(&Config{
		VerifyTimeout:   250,
		HeartInterval:   500,
		NetLagSampleNum: 10,
		Splitnum:        128,
	}, unixclienthandleVerify, unixclienthandleonline, unixclienthandleuserdata, unixclienthandleoffline)
	go func() {
		for count := 0; count < 10000; count++ {
			unixclientinstance.StartUnixsocketClient(fmt.Sprintf("unixclient%d", count), []byte{}, "./test.socket")
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func unixclienthandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func unixclienthandleonline(p *Peer, peername string, uniqueid int64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func unixclienthandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
}

func unixclienthandleoffline(p *Peer, peername string, uniqueid int64) {
}
