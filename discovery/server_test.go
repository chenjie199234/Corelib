package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Server1(t *testing.T) {
	instance := NewDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "server1",
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500 * time.Millisecond,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'})
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if instance != nil {
				instance.lker.RLock()
				fmt.Println(instance.allapps)
				instance.lker.RUnlock()
			}
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9234")
}
func Test_Server2(t *testing.T) {
	instance := NewDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "server2",
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500 * time.Millisecond,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'})
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if instance != nil {
				instance.lker.RLock()
				fmt.Println(instance.allapps)
				instance.lker.RUnlock()
			}
		}
	}()
	instance.StartDiscoveryServer("127.0.0.1:9235")
}
