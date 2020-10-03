package test

import (
	"context"
	//"fmt"
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
}

func Test_Appserver1(t *testing.T) {
	tcpconfig := &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  4096,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello1,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, tcpconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8888,
	})
	server.StartMrpcServer(tcpconfig, "127.0.0.1:8888")
}
func Hello1(ctx context.Context, req *HelloReq) (*HelloResp, *mrpc.MsgErr) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	return &HelloResp{Name: "server1", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver2(t *testing.T) {
	tcpconfig := &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  4096,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello2,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, tcpconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8889,
	})
	server.StartMrpcServer(tcpconfig, "127.0.0.1:8889")
}
func Hello2(ctx context.Context, req *HelloReq) (*HelloResp, *mrpc.MsgErr) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	return &HelloResp{Name: "server2", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver3(t *testing.T) {
	tcpconfig := &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  4096,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello3,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, tcpconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8890,
	})
	server.StartMrpcServer(tcpconfig, "127.0.0.1:8890")
}
func Hello3(ctx context.Context, req *HelloReq) (*HelloResp, *mrpc.MsgErr) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	return &HelloResp{Name: "server3", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
func Test_Appserver4(t *testing.T) {
	tcpconfig := &stream.TcpConfig{
		ConnectTimeout:       1000,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  4096,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}
	verifydata := []byte("test")
	server := mrpc.NewMrpcServer(serverinstanceconfig, verifydata)
	RegisterMrpcTestService(server, &MrpcTestService{
		Midware: nil,
		Hello:   Hello4,
	})
	discovery.NewDiscoveryClient(serverinstanceconfig, tcpconfig, verifydata, "http://127.0.0.1:8080/discoveryservers")
	discovery.RegisterSelf(&discovery.RegMsg{
		TcpIp:   "127.0.0.1",
		TcpPort: 8891,
	})
	server.StartMrpcServer(tcpconfig, "127.0.0.1:8891")
}
func Hello4(ctx context.Context, req *HelloReq) (*HelloResp, *mrpc.MsgErr) {
	//fmt.Println(trace.GetTrace(ctx))
	//fmt.Println(mrpc.GetAllMetadata(ctx))
	return &HelloResp{Name: "server4", Sex: 1, Addr: "space", Tel: "123456789"}, nil
}
