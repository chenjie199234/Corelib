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
	}, "default", "discoverycenter1", []byte{'t', 'e', 's', 't'})
	if e != nil {
		panic(e)
	}
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if instance.groups["testgroup.testclient1"] != nil {
				fmt.Println("client1:", " apps:", instance.groups["testgroup.testclient1"].apps, " bewatched:", instance.groups["testgroup.testclient1"].bewatched)
			} else {
				fmt.Println("client1: nil")
			}
			if instance.groups["testgroup.testclient2"] != nil {
				fmt.Println("client2:", " apps:", instance.groups["testgroup.testclient2"].apps, " bewatched:", instance.groups["testgroup.testclient2"].bewatched)
			} else {
				fmt.Println("client2: nil")
			}
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9234")
}
func Test_Server2(t *testing.T) {
	instance, _ := NewDiscoveryServer(&stream.InstanceConfig{
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:         500 * time.Millisecond,
			SocketRBufLen:          1024,
			SocketWBufLen:          1024,
			MaxBufferedWriteMsgNum: 256,
		},
	}, "default", "discoverycenter2", []byte{'t', 'e', 's', 't'})
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if instance != nil {
				if instance.groups["testgroup.testclient1"] != nil {
					fmt.Println("client1:", " apps:", instance.groups["testgroup.testclient1"].apps, " bewatched:", instance.groups["testgroup.testclient1"].bewatched)
				} else {
					fmt.Println("client1: nil")
				}
				if instance.groups["testgroup.testclient2"] != nil {
					fmt.Println("client2:", " apps:", instance.groups["testgroup.testclient2"].apps, " bewatched:", instance.groups["testgroup.testclient2"].bewatched)
				} else {
					fmt.Println("client2: nil")
				}
			}
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9235")
}
