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
				NetLagSampleNum:    10,
				GroupNum:           10,
				Verifyfunc:         unixclienthandleVerify,
				Onlinefunc:         unixclienthandleonline,
				Userdatafunc:       unixclienthandleuserdata,
				Offlinefunc:        unixclienthandleoffline,
			})
			unixclientinstance.StartUnixsocketClient(&UnixConfig{
				ConnectTimeout:       1000,
				SocketReadBufferLen:  1024,
				SocketWriteBufferLen: 1024,
				AppMinReadBufferLen:  1024,
				AppMaxReadBufferLen:  65535,
				AppWriteBufferNum:    256,
			}, "./test.socket", []byte{'t', 'e', 's', 't', 'c'})
			time.Sleep(time.Millisecond)
		}
	}()
	http.ListenAndServe(":8083", nil)
}
func unixclienthandleVerify(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return nil, true
}

var unix int64

func unixclienthandleonline(p *Peer, peeruniquename string, uniqueid uint64) {
	old := atomic.SwapInt64(&unix, 1)
	if old == 0 {
		go func() {
			for {
				time.Sleep(time.Second)
				lag := p.GetAverageNetLag()
				fmt.Println(float64(lag)/1000.0/1000.0, "ms")
			}
		}()
	}
}

func unixclienthandleuserdata(p *Peer, peeruniquename string, uniqueid uint64, data []byte) {
	fmt.Printf("%s\n", data)
}

func unixclienthandleoffline(p *Peer, peeruniquename string, uniqueid uint64) {
}
