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

//var unixclientinstance *Instance

func Test_Unixclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			unixclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("unixclient%d", count),
				VerifyTimeout:      500,
				VerifyData:         []byte{'t', 'e', 's', 't'},
				HeartbeatTimeout:   1500,
				HeartprobeInterval: 500,
				NetLagSampleNum:    10,
				GroupNum:           10,
				Verifyfunc:         unixclienthandleVerify,
				Onlinefunc:         unixclienthandleonline,
				Userdatafunc:       unixclienthandleuserdata,
				Offlinefunc:        unixclienthandleoffline,
			})
			unixclientinstance.StartUnixsocketClient(&UnixConfig{
				ConnectTimeout:       1000,
				SocketReadBufferLen:  1024,
				SocketWriteBufferLen: 1024,
				AppMinReadBufferLen:  1024,
				AppMaxReadBufferLen:  65535,
				AppWriteBufferNum:    256,
			}, "./test.socket")
			if count == 0 {
				go func() {
					for {
						time.Sleep(time.Second)
						lag, e := unixclientinstance.GetAverageNetLag("server")
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
	http.ListenAndServe(":8083", nil)
}
func unixclienthandleVerify(ctx context.Context, selfname string, selfVerifyData []byte, peername string, peerVerifyData []byte) bool {
	return true
}

func unixclienthandleonline(p *Peer, peername string, uniqueid uint64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func unixclienthandleuserdata(ctx context.Context, p *Peer, peername string, uniqueid uint64, data []byte) {
	fmt.Printf("%s\n", data)
}

func unixclienthandleoffline(p *Peer, peername string, uniqueid uint64) {
}
