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
		for count := 0; count < 1; count++ {
			unixclientinstance, _ := NewInstance(&InstanceConfig{
				HeartbeatTimeout:   1500 * time.Millisecond,
				HeartprobeInterval: 500 * time.Millisecond,
				GroupNum:           10,
				Verifyfunc:         unixclienthandleVerify,
				Onlinefunc:         unixclienthandleonline,
				Userdatafunc:       unixclienthandleuserdata,
				Offlinefunc:        unixclienthandleoffline,
			}, "testgroup", "unixclient")
			unixclientinstance.StartUnixClient("./test.socket", []byte{'t', 'e', 's', 't', 'c'})
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

var firstunixclient int64

func unixclienthandleonline(p *Peer, peeruniquename string, starttime int64) {
	if atomic.SwapInt64(&firstunixclient, 1) == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage([]byte{'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd'}, starttime, true)
			}
		}()
	}
}

func unixclienthandleuserdata(p *Peer, peeruniquename string, data []byte, starttime int64) {
	fmt.Printf("%s\n", data)
}

func unixclienthandleoffline(p *Peer, peeruniquename string) {
}
