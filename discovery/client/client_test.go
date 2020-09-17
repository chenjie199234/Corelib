package client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/discovery/msg"
	"github.com/chenjie199234/Corelib/stream"
)

func Test_Client(t *testing.T) {
	go func() {
		http.HandleFunc("/discoveryservers", func(w http.ResponseWriter, r *http.Request) {
			d, _ := json.Marshal([]string{"server:127.0.0.1:9234"})
			w.WriteHeader(200)
			w.Write(d)
			return
		})
		http.ListenAndServe("127.0.0.1:8080", nil)
	}()
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if instance != nil {
				instance.lker.RLock()
				if server, ok := instance.servers["server:127.0.0.1:9234"]; ok {
					fmt.Println(hex.EncodeToString(server.htree.GetRootHash()))
				}
				instance.lker.RUnlock()
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
	}, []byte{'t', 'e', 's', 't'}, &msg.RegMsg{
		GrpcAddr: "127.0.0.1:9000",
		HttpAddr: "127.0.0.1:8000",
		TcpAddr:  "127.0.0.1:7000",
	}, "http://127.0.0.1:8080/discoveryservers")
}
