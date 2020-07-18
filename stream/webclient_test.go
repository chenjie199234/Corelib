package stream

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

//var webclientinstance *Instance

func Test_Webclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			webclientinstance := NewInstance(&Config{
				SelfName:        fmt.Sprintf("webclient%d", count),
				VerifyTimeout:   500,
				HeartTimeout:    1000,
				NetLagSampleNum: 10,
				Splitnum:        10,
			}, webclienthandleVerify, webclienthandleonline, webclienthandleuserdata, webclienthandleoffline)
			webclientinstance.StartWebsocketClient([]byte{}, "ws://127.0.0.1:9234/test")
			if count == 0 {
				go func() {
					for {
						time.Sleep(time.Second)
						lag, e := webclientinstance.GetAverageNetLag("server")
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
func webclienthandleVerify(selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func webclienthandleonline(p *Peer, peername string, uniqueid int64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func webclienthandleuserdata(p *Peer, peername string, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
}

func webclienthandleoffline(p *Peer, peername string, uniqueid int64) {
}
