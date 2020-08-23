package stream

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

var webclientinstance *Instance

func Test_Webclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			webclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("webclient%d", count),
				VerifyTimeout:      500,
				VerifyData:         []byte{'t', 'e', 's', 't'},
				HeartbeatTimeout:   1500,
				HeartprobeInterval: 500,
				NetLagSampleNum:    10,
				GroupNum:           10,
				Verifyfunc:         webclienthandleVerify,
				Onlinefunc:         webclienthandleonline,
				Userdatafunc:       webclienthandleuserdata,
				Offlinefunc:        webclienthandleoffline,
			})
			webclientinstance.StartWebsocketClient(&WebConfig{
				ConnectTimeout:       1000,
				HttpMaxHeaderLen:     1024,
				SocketReadBufferLen:  1024,
				SocketWriteBufferLen: 1024,
				AppWriteBufferNum:    256,
			}, "ws://127.0.0.1:9235/test")
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
	http.ListenAndServe(":8085", nil)
}
func webclienthandleVerify(ctx context.Context, selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func webclienthandleonline(p *Peer, peername string, uniqueid uint64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func webclienthandleuserdata(ctx context.Context, p *Peer, peername string, uniqueid uint64, data []byte) {
	fmt.Printf("%s\n", data)
}

func webclienthandleoffline(p *Peer, peername string, uniqueid uint64) {
}
