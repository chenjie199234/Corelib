package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"testing"
	"time"
)

var wsserverinstance *Instance

func Test_Wsserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	wsserverinstance, _ = NewInstance(&InstanceConfig{
		HeartbeatTimeout:   1500 * time.Millisecond,
		HeartprobeInterval: 500 * time.Millisecond,
		RecvIdleTimeout:    30 * time.Second, //30s
		GroupNum:           10,
		Verifyfunc:         wsserverhandleVerify,
		Onlinefunc:         wsserverhandleonline,
		Userdatafunc:       wsserverhandleuserdata,
		Offlinefunc:        wsserverhandleoffline,
	}, "testgroup", "wsserver")
	go wsserverinstance.StartWsServer("127.0.0.1:9234", "/ws")
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", wsserverinstance.totalpeernum)
		}
	}()
	go func() {
		time.Sleep(time.Minute)
		wsserverinstance.Stop()
		fmt.Println("stop:", wsserverinstance.totalpeernum)
	}()
	http.ListenAndServe(":8080", nil)
}
func wsserverhandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	index := strings.Index(peeruniquename, ":")
	if peeruniquename[:index] != "testgroup.wsclient" {
		panic("name error")
	}
	if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return []byte{'t', 'e', 's', 't'}, true
}
func wsserverhandleonline(p *Peer, peeruniquename string, starttime int64) {
}
func wsserverhandleuserdata(p *Peer, peeruniquename string, data []byte, starttime int64) {
	fmt.Printf("%s:%s\n", peeruniquename, data)
	p.SendMessage(data, starttime, true)
}
func wsserverhandleoffline(p *Peer, peeruniquename string) {
}
