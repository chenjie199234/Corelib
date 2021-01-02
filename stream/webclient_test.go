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

var webclientinstance *Instance

func Test_Webclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			webclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("webclient%d", count),
				VerifyTimeout:      500,
				HeartbeatTimeout:   1500,
				HeartprobeInterval: 500,
				GroupNum:           10,
				Verifyfunc:         webclienthandleVerify,
				Onlinefunc:         webclienthandleonline,
				Userdatafunc:       webclienthandleuserdata,
				Offlinefunc:        webclienthandleoffline,
			})
			webclientinstance.StartWebsocketClient("ws://127.0.0.1:9235/test", []byte{'t', 'e', 's', 't', 'c'})
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8085", nil)
}
func webclienthandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var web int64

func webclienthandleonline(p *Peer, peeruniquename string, starttime uint64) {
	old := atomic.SwapInt64(&web, 1)
	if old == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage([]byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'}, starttime, true)
			}
		}()
	}
}

func webclienthandleuserdata(p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s\n", data)
}

func webclienthandleoffline(p *Peer, peeruniquename string, starttime uint64) {
}
