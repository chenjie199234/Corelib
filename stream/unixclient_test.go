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

//var unixclientinstance *Instance

func Test_Unixclient(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	go func() {
		for count := 0; count < 10000; count++ {
			unixclientinstance := NewInstance(&InstanceConfig{
				SelfName:           fmt.Sprintf("unixclient%d", count),
				VerifyTimeout:      500,
				HeartbeatTimeout:   1500,
				HeartprobeInterval: 500,
				GroupNum:           10,
				Verifyfunc:         unixclienthandleVerify,
				Onlinefunc:         unixclienthandleonline,
				Userdatafunc:       unixclienthandleuserdata,
				Offlinefunc:        unixclienthandleoffline,
			})
			unixclientinstance.StartUnixsocketClient("./test.socket", []byte{'t', 'e', 's', 't', 'c'})
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8083", nil)
}
func unixclienthandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var unix int64

func unixclienthandleonline(p *Peer, peeruniquename string, starttime uint64) {
	old := atomic.SwapInt64(&unix, 1)
	if old == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage([]byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'}, starttime)
			}
		}()
	}
}

func unixclienthandleuserdata(p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s\n", data)
}

func unixclienthandleoffline(p *Peer, peeruniquename string, starttime uint64) {
}
