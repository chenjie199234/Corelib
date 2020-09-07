package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

func Test_Tcpclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			tcpclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("tcpclient%d", count),
				VerifyTimeout:      500,
				HeartbeatTimeout:   1500,
				HeartprobeInterval: 500,
				NetLagSampleNum:    10,
				GroupNum:           10,
				Verifyfunc:         tcpclienthandleVerify,
				Onlinefunc:         tcpclienthandleonline,
				Userdatafunc:       tcpclienthandleuserdata,
				Offlinefunc:        tcpclienthandleoffline,
			})
			tcpclientinstance.StartTcpClient(&TcpConfig{
				ConnectTimeout:       1000,
				SocketReadBufferLen:  1024,
				SocketWriteBufferLen: 1024,
				AppMinReadBufferLen:  1024,
				AppMaxReadBufferLen:  65535,
				AppWriteBufferNum:    256,
			}, "127.0.0.1:9234")
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
func tcpclienthandleVerify(ctx context.Context, peername string, peerVerifyData []byte) []byte {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
	}
	return nil
}

func tcpclienthandleonline(p *Peer, peername string, uniqueid uint64) {
	//go func() {
	//        for {
	//                fmt.Println(peername)
	//                time.Sleep(time.Second)
	//                p.SendMessage([]byte("hello world!"), uniqueid)
	//        }
	//}()
}

func tcpclienthandleuserdata(ctx context.Context, p *Peer, peername string, uniqueid uint64, data []byte) {
	fmt.Printf("%s\n", data)
}

func tcpclienthandleoffline(p *Peer, peername string, uniqueid uint64) {
}
