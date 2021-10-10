package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func Test_Tcpclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			tcpclientinstance, _ := NewInstance(&InstanceConfig{
				HeartprobeInterval: time.Second,
				GroupNum:           1,
				Verifyfunc:         tcpclienthandleVerify,
				Onlinefunc:         tcpclienthandleonline,
				PingPongFunc:       tcpclientpingpong,
				Userdatafunc:       tcpclienthandleuserdata,
				Offlinefunc:        tcpclienthandleoffline,
			}, "testgroup", "tcpclient")
			tcpclientinstance.StartTcpClient("127.0.0.1:9234", []byte{'t', 'e', 's', 't', 'c'}, nil)
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8081", nil)
}
func tcpclienthandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	index := strings.Index(peeruniquename, ":")
	if peeruniquename[:index] != "testgroup.tcpserver" {
		panic("name error")
	}
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var firsttcpclient int64
var firsttcpclientpeer *Peer

func tcpclienthandleonline(p *Peer) bool {
	if atomic.SwapInt64(&firsttcpclient, 1) == 0 {
		firsttcpclientpeer = p
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage(nil, bytes.Repeat([]byte{'a'}, 10))
			}
		}()
	}
	return true
}

var firsttcpclientpingpong int64

func tcpclientpingpong(p *Peer) {
	if p == firsttcpclientpeer {
		fmt.Println("ping pong:", p.GetPeerNetlag())
	}
}
func tcpclienthandleuserdata(p *Peer, data []byte) {
	fmt.Printf("%s\n", data)
}

func tcpclienthandleoffline(p *Peer) {
}
