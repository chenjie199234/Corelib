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

var tcpserverinstance *Instance

func Test_Tcpserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	tcpserverinstance, _ = NewInstance(&InstanceConfig{
		HeartprobeInterval: time.Second,
		RecvIdleTimeout:    30 * time.Second, //30s
		GroupNum:           10,
		Verifyfunc:         tcpserverhandleVerify,
		Onlinefunc:         tcpserverhandleonline,
		Userdatafunc:       tcpserverhandleuserdata,
		Offlinefunc:        tcpserverhandleoffline,
	}, "testgroup", "tcpserver")
	go tcpserverinstance.StartTcpServer("127.0.0.1:9234", nil)
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", tcpserverinstance.GetPeerNum())
		}
	}()
	go func() {
		time.Sleep(time.Minute)
		tcpserverinstance.Stop()
		fmt.Println("stop:", tcpserverinstance.GetPeerNum())
	}()
	http.ListenAndServe(":8080", nil)
}
func tcpserverhandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
	index := strings.Index(peeruniquename, ":")
	if peeruniquename[:index] != "testgroup.tcpclient" {
		panic("name error")
	}
	if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return []byte{'t', 'e', 's', 't'}, true
}
func tcpserverhandleonline(p *Peer, peeruniquename string, starttime int64) bool {
	return true
}
func tcpserverhandleuserdata(p *Peer, peeruniquename string, data []byte, starttime int64) {
	fmt.Printf("%s:%s\n", peeruniquename, data)
	p.SendMessage(data, starttime, true)
}
func tcpserverhandleoffline(p *Peer, peeruniquename string, _ [][]byte) {
}
