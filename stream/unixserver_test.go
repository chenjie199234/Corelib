package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"testing"
	"time"
)

var unixserverinstance *Instance

func Test_Unixserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	unixserverinstance = NewInstance(&InstanceConfig{
		SelfName:           "server",
		HeartbeatTimeout:   1500 * time.Millisecond,
		HeartprobeInterval: 500 * time.Millisecond,
		RecvIdleTimeout:    30 * time.Second, //30s
		GroupNum:           10,
		Verifyfunc:         unixserverhandleVerify,
		Onlinefunc:         unixserverhandleonline,
		Userdatafunc:       unixclienthandleuserdata,
		Offlinefunc:        unixclienthandleoffline,
	})
	os.Remove("./test.socket")
	go unixserverinstance.StartUnixServer("./test.socket")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", unixserverinstance.totalpeernum)
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
}
func unixserverhandleuserdata(ctx context.Context, p *Peer, peeruniquename string, data []byte, starttime uint64) {
	fmt.Printf("%s:%s\n", peeruniquename, data)
	p.SendMessage(data, starttime, true)
}
func unixserverhandleoffline(p *Peer, peeruniquename string, starttime uint64) {
}
