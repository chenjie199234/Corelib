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

func Test_Wsclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			tcpclientinstance, _ := NewInstance(&InstanceConfig{
				HeartprobeInterval: time.Second,
				GroupNum:           1,
				TcpC:               &TcpConfig{
					//MaxMsgLen: 65535,
				},
				VerifyFunc:   tcpclienthandleVerify,
				OnlineFunc:   tcpclienthandleonline,
				PingPongFunc: tcpclientpingpong,
				UserdataFunc: tcpclienthandleuserdata,
				OfflineFunc:  tcpclienthandleoffline,
			})
			tcpclientinstance.StartClient("ws://127.0.0.1:9234", []byte{'t', 'e', 's', 't', 'c'}, nil)
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8082", nil)
}
func wsclienthandleVerify(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var firstwsclient int64
var firstwsclientpeer *Peer

func wsclienthandleonline(p *Peer) bool {
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

var firstwsclientpingpong int64

func wsclientpingpong(p *Peer) {
	if p == firsttcpclientpeer {
		fmt.Println("ping pong:", p.GetNetlag())
	}
}
func wsclienthandleuserdata(p *Peer, data []byte) {
	fmt.Printf("%s:%d\n", p.c.RemoteAddr().String(), len(data))
}

func wsclienthandleoffline(p *Peer) {
}
