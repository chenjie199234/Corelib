package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

//var unixclientinstance *Instance

func Test_Unixclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			unixclientinstance := NewInstance(&Config{
				SelfName:        fmt.Sprintf("unixclient%d", count),
				VerifyTimeout:   500,
				HeartTimeout:    1000,
				NetLagSampleNum: 10,
				Splitnum:        10,
			}, unixclienthandleVerify, unixclienthandleonline, unixclienthandleuserdata, unixclienthandleoffline)
			unixclientinstance.StartUnixsocketClient([]byte{}, "./test.socket")
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
