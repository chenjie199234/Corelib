package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

//var tcpclientinstance *Instance

func Test_Tcpclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			tcpclientinstance := NewInstance(&Config{
				SelfName:        fmt.Sprintf("tcpclient%d", count),
				VerifyTimeout:   500,
				HeartTimeout:    1000,
				NetLagSampleNum: 10,
				Splitnum:        10,
			}, tcpclienthandleVerify, tcpclienthandleonline, tcpclienthandleuserdata, tcpclienthandleoffline)
			tcpclientinstance.StartTcpClient([]byte{}, "127.0.0.1:9234")
			if count == 0 {
				go func() {
					for {
						time.Sleep(time.Second)
						lag, e := tcpclientinstance.GetAverageNetLag("server")
						if e != nil {
							fmt.Println(e)
						} else {
							fmt.Println(float64(lag)/1000.0/1000.0, "ms")
						}
					}
				}()
			}
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
