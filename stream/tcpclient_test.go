package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

var tcpclientinstance *Instance

func Test_Tcpclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	tcpclientinstance = NewInstance(&Config{
		VerifyTimeout:   500,
		HeartInterval:   1000,
		NetLagSampleNum: 10,
		Splitnum:        10,
	}, tcpclienthandleVerify, tcpclienthandleonline, tcpclienthandleuserdata, tcpclienthandleoffline)
	go func() {
		for count := 0; count < 10000; count++ {
			tcpclientinstance.StartTcpClient(fmt.Sprintf("tcpclient%d", count), []byte{}, "127.0.0.1:9234")
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func tcpclienthandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func tcpclienthandleonline(p *Peer, peername string, uniqueid int64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func tcpclienthandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
}

func tcpclienthandleoffline(p *Peer, peername string, uniqueid int64) {
}
