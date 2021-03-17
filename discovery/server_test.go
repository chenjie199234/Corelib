package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Server1(t *testing.T) {
	instance, e := NewDiscoveryServer(&stream.InstanceConfig{
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:         500 * time.Millisecond,
			SocketRBufLen:          1024,
			SocketWBufLen:          1024,
			MaxBufferedWriteMsgNum: 256,
		},
	}, "default", "discoverycenter1", "", "test")
	if e != nil {
		panic(e)
	}
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			fmt.Println("client1:", instance.GetAppInfo("testgroup.testclient1")["testgroup.testclient1"])
			fmt.Println("client2:", instance.GetAppInfo("testgroup.testclient2")["testgroup.testclient2"])
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9234")
}
func Test_Server2(t *testing.T) {
	instance, e := NewDiscoveryServer(&stream.InstanceConfig{
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:         500 * time.Millisecond,
			SocketRBufLen:          1024,
			SocketWBufLen:          1024,
			MaxBufferedWriteMsgNum: 256,
		},
	}, "default", "discoverycenter2", "", "test")
	if e != nil {
		panic(e)
	}
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			fmt.Println("client1:", instance.GetAppInfo("testgroup.testclient1")["testgroup.testclient1"])
			fmt.Println("client2:", instance.GetAppInfo("testgroup.testclient2")["testgroup.testclient2"])
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9235")
}
