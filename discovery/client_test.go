package discovery

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Client(t *testing.T) {
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if clientinstance != nil {
				clientinstance.slker.RLock()
				if server, ok := clientinstance.servers["server1:127.0.0.1:9234"]; ok {
					fmt.Println("server1:" + hex.EncodeToString(server.htree.GetRootHash()))
				}
				if server, ok := clientinstance.servers["server2:127.0.0.1:9235"]; ok {
					fmt.Println("server2:" + hex.EncodeToString(server.htree.GetRootHash()))
				}
				clientinstance.slker.RUnlock()
			}
		}
	}()
	StartDiscoveryClient(&stream.InstanceConfig{
		SelfName:           "client",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		NetLagSampleNum:    10,
		GroupNum:           1,
	}, &stream.TcpConfig{
		ConnectTimeout:       500,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, []byte{'t', 'e', 's', 't'}, &RegMsg{
		GrpcAddr: "127.0.0.1:9000",
		HttpAddr: "127.0.0.1:8000",
		TcpAddr:  "127.0.0.1:7000",
	}, "http://127.0.0.1:8080/discoveryservers")
}
