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
				HeartbeatTimeout:   1500 * time.Millisecond,
				HeartprobeInterval: 500 * time.Millisecond,
				GroupNum:           1,
				Verifyfunc:         tcpclienthandleVerify,
				Onlinefunc:         tcpclienthandleonline,
				Userdatafunc:       tcpclienthandleuserdata,
				Offlinefunc:        tcpclienthandleoffline,
			}, "testgroup", "tcpclient")
			tcpclientinstance.StartTcpClient("127.0.0.1:9234", []byte{'t', 'e', 's', 't', 'c'})
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

func tcpclienthandleonline(p *Peer, peeruniquename string, starttime int64) bool {
	if atomic.SwapInt64(&firsttcpclient, 1) == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				p.SendMessage(bytes.Repeat([]byte{'a'}, 1100), starttime, true)
			}
		}()
	}
	return true
}

func tcpclienthandleuserdata(p *Peer, peeruniquename string, data []byte, starttime int64) {
	fmt.Printf("%s\n", data)
}

func tcpclienthandleoffline(p *Peer, peeruniquename string) {
}
