package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var unixserverinstance *Instance
var unixcount int64

func Test_Unixserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	unixserverinstance = NewInstance(&InstanceConfig{
		SelfName:           "server",
		VerifyTimeout:      500,
		HeartbeatTimeout:   1500,
		HeartprobeInterval: 500,
		RecvIdleTimeout:    30000, //30s
		GroupNum:           10,
		Verifyfunc:         unixserverhandleVerify,
		Onlinefunc:         unixserverhandleonline,
		Userdatafunc:       unixclienthandleuserdata,
		Offlinefunc:        unixclienthandleoffline,
	})
	os.Remove("./test.socket")
	go unixserverinstance.StartUnixsocketServer("./test.socket")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", unixcount)
		}
	}()
	http.ListenAndServe(":8082", nil)
}
func unixserverhandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return []byte{'t', 'e', 's', 't'}, true
}
func unixserverhandleonline(p *Peer, peeruniquename string, starttime uint64) {
	atomic.AddInt64(&unixcount, 1)
}
func unixserverhandleuserdata(ctx context.Context, p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s:%s\n", peeruniquename, data)
	p.SendMessage(data, starttime)
}
func unixserverhandleoffline(p *Peer, peeruniquename string, starttime uint64) {
	atomic.AddInt64(&unixcount, -1)
}
