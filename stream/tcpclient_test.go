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
			tcpclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("tcpclient%d", count),
				VerifyTimeout:      500,
				VerifyData:         []byte{'t', 'e', 's', 't'},
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
