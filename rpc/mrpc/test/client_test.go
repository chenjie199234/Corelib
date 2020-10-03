package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/rpc/mrpc"
	"github.com/chenjie199234/Corelib/stream"
)

var clientinstanceconfig *stream.InstanceConfig = &stream.InstanceConfig{
	SelfName:           "appclient",
	VerifyTimeout:      1000,
	HeartbeatTimeout:   3000,
	HeartprobeInterval: 1000,
	GroupNum:           1,
}

var api *MrpcTestClient

func Test_Client(t *testing.T) {
	tcpconfig := &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  4096,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}
	verifydata := []byte("test")
	discovery.NewDiscoveryClient(clientinstanceconfig, tcpconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	client := mrpc.NewMrpcClient(clientinstanceconfig, tcpconfig, "appserver", verifydata, pick)
	api = NewMrpcTestClient(client)
	call()
	select {}
}

var count = 10000

func call() {
	conder := sync.NewCond(&sync.Mutex{})
	ch := make(chan struct{}, 10000)
	f := func() {
		conder.L.Lock()
		ch <- struct{}{}
		conder.Wait()
		ctx := context.Background()
		ctx = mrpc.SetAllMetadata(ctx, map[string]string{"req": "req"})
		_, e := api.Hello(ctx, &HelloReq{
			Name: "client",
			Sex:  0,
			Addr: "computer",
			Tel:  "123456789",
		})
		if e != nil {
			panic(fmt.Sprintf("code:%d msg:%s", e.Code, e.Msg))
		}
		conder.L.Unlock()
		ch <- struct{}{}
	}
	for i := 0; i < count; i++ {
		go f()
	}
	for i := 0; i < count; i++ {
		<-ch
	}
	start := time.Now().UnixNano()
	conder.Broadcast()
	for i := 0; i < count; i++ {
		<-ch
	}
	end := time.Now().UnixNano()
	fmt.Println(float64(end-start) / 1000.0 / 1000.0)
	fmt.Println(float64(count) / (float64(end-start) / 1000.0 / 1000.0 / 1000.0))
}
func pick(pickinfos map[*mrpc.Serverinfo]mrpc.PickInfo) *mrpc.Serverinfo {
	for k := range pickinfos {
		return k
	}
	return nil
}
