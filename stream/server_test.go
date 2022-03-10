package stream

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"testing"
	"time"
)

var serverinstance *Instance

func Test_Tcpserver(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	serverinstance, _ = NewInstance(&InstanceConfig{
		HeartprobeInterval: time.Second,
		RecvIdleTimeout:    30 * time.Second, //30s
		GroupNum:           10,
		TcpC:               &TcpConfig{
			//MaxMsgLen: 6553500,
		},
		VerifyFunc:   serverhandleVerify,
		OnlineFunc:   serverhandleonline,
		UserdataFunc: serverhandleuserdata,
		OfflineFunc:  serverhandleoffline,
	})
	go serverinstance.StartServer("127.0.0.1:9234", nil)
	go func() {
		for {
			time.Sleep(time.Second)
			fmt.Println("client num:", serverinstance.GetPeerNum())
		}
	}()
	go func() {
		time.Sleep(time.Minute)
		serverinstance.Stop()
		fmt.Println("stop:", serverinstance.GetPeerNum())
	}()
	http.ListenAndServe(":8080", nil)
}
func serverhandleVerify(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
		fmt.Println("verify error")
		return nil, false
	}
	return []byte{'t', 'e', 's', 't'}, true
}
func serverhandleonline(p *Peer) bool {
	return true
}
func serverhandleuserdata(p *Peer, data []byte) {
	fmt.Printf("%s:%d\n", p.c.RemoteAddr().String(), len(data))
	back := make([]byte, len(data))
	copy(back, data)
	if e := p.SendMessage(nil, back, nil, nil); e != nil {
		fmt.Println(e)
	}
}
func serverhandleoffline(p *Peer) {
}
