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
		for range 10000 {
			tcpclientinstance, _ := NewInstance(&InstanceConfig{
				HeartprobeInterval: time.Second,
				TcpC:               &TcpConfig{
					//MaxMsgLen: 65535,
				},
				VerifyFunc:   tcpclienthandleVerify,
				OnlineFunc:   tcpclienthandleonline,
				PingPongFunc: tcpclientpingpong,
				UserdataFunc: tcpclienthandleuserdata,
				OfflineFunc:  tcpclienthandleoffline,
			})
			tcpclientinstance.StartClient("127.0.0.1:9234", false, []byte{'t', 'e', 's', 't', 'c'}, nil)
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func tcpclienthandleVerify(ctx context.Context, peerVerifyData []byte) ([]byte, string, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, "", false
	}
	return nil, "", true
}

var firsttcpclient int64
var firsttcpclientpeer *Peer

func tcpclienthandleonline(ctx context.Context, p *Peer) bool {
	if atomic.SwapInt64(&firsttcpclient, 1) == 0 {
		firsttcpclientpeer = p
		go func() {
			for {
				time.Sleep(time.Second)
				if e := p.SendMessage(nil, bytes.Repeat([]byte{'a'}, 1024000), nil, nil); e != nil {
					fmt.Println(e)
				}
			}
		}()
	}
	return true
}

var firsttcpclientpingpong int64

func tcpclientpingpong(p *Peer) {
	if p == firsttcpclientpeer {
		fmt.Println("ping pong:", p.GetNetlag())
	}
}
func tcpclienthandleuserdata(p *Peer, data []byte) {
	fmt.Printf("%s:%d\n", p.c.RemoteAddr().String(), len(data))
}

func tcpclienthandleoffline(p *Peer) {
}
