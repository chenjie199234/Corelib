package discovery

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Server(t *testing.T) {
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if serverinstance != nil {
				fmt.Println(hex.EncodeToString(serverinstance.htree.GetRootHash()))
			}
		}
	}()
	StartDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "server",
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
	}, "127.0.0.1:9234", []byte{'t', 'e', 's', 't'})
}
