package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync/atomic"
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
			}, "127.0.0.1:9234", []byte{'t', 'e', 's', 't', 'c'})
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func tcpclienthandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var tcp int64

func tcpclienthandleonline(p *Peer, peeruniquename string, starttime uint64) {
	old := atomic.SwapInt64(&tcp, 1)
	if old == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage([]byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'}, starttime)
			}
		}()
	}
}

func tcpclienthandleuserdata(p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s\n", data)
}

func tcpclienthandleoffline(p *Peer, peeruniquename string) {
}
