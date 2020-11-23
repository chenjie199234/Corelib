package test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/rpc/mrpc"
	"github.com/chenjie199234/Corelib/stream"
	//"github.com/chenjie199234/Corelib/sys/trace"
)

var serverinstanceconfig *stream.InstanceConfig = &stream.InstanceConfig{
	SelfName:           "appserver",
	VerifyTimeout:      1000,
	HeartbeatTimeout:   3000,
	HeartprobeInterval: 1000,
	GroupNum:           1,
	TcpC: &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppWriteBufferNum:    256,
	},
}

func Test_Appserver1(t *testing.T) {

	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello1,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8888,
	})
	server.StartMrpcServer("127.0.0.1:8888")
}

var count1 int32

func Hello1(ctx context.Context, req *HelloReq) (*HelloResp, error) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	fmt.Println(atomic.AddInt32(&count1, 1))
	return &HelloResp{Name: "server1", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver2(t *testing.T) {
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello2,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8889,
	})
	server.StartMrpcServer("127.0.0.1:8889")
}

var count2 int32

func Hello2(ctx context.Context, req *HelloReq) (*HelloResp, error) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	fmt.Println(atomic.AddInt32(&count2, 1))
	return &HelloResp{Name: "server2", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver3(t *testing.T) {
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello3,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8890,
	})
	server.StartMrpcServer("127.0.0.1:8890")
}

var count3 int32

func Hello3(ctx context.Context, req *HelloReq) (*HelloResp, error) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	fmt.Println(atomic.AddInt32(&count3, 1))
	return &HelloResp{Name: "server3", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver4(t *testing.T) {
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello4,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8891,
	})
	server.StartMrpcServer("127.0.0.1:8891")
}

var count4 int32

func Hello4(ctx context.Context, req *HelloReq) (*HelloResp, error) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	fmt.Println(atomic.AddInt32(&count4, 1))
	return &HelloResp{Name: "server4", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
