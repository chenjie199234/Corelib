package discovery

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Server1(t *testing.T) {
	go func() {
		http.HandleFunc("/discoveryservers", func(w http.ResponseWriter, r *http.Request) {
			d, _ := json.Marshal([]string{"server1:127.0.0.1:9234", "server2:127.0.0.1:9235"})
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
			if serverinstance != nil {
				fmt.Println(hex.EncodeToString(serverinstance.htree.GetRootHash()))
			}
		}
	}()
	NewDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "server1",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		NetLagSampleNum:    10,
		GroupNum:           1,
	}, []byte{'t', 'e', 's', 't'})
	StartDiscoveryServer(&stream.TcpConfig{
		ConnectTimeout:       500,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, "127.0.0.1:9234")
}
func Test_Server2(t *testing.T) {
	go func() {
		http.HandleFunc("/discoveryservers", func(w http.ResponseWriter, r *http.Request) {
			d, _ := json.Marshal([]string{"server1:127.0.0.1:9234", "server2:127.0.0.1:9235"})
			w.WriteHeader(200)
			w.Write(d)
			return
		})
		http.ListenAndServe("127.0.0.1:8081", nil)
	}()
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if serverinstance != nil {
				fmt.Println(hex.EncodeToString(serverinstance.htree.GetRootHash()))
			}
		}
	}()
	NewDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "server2",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		NetLagSampleNum:    10,
		GroupNum:           1,
	}, []byte{'t', 'e', 's', 't'})
	StartDiscoveryServer(&stream.TcpConfig{
		ConnectTimeout:       500,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, "127.0.0.1:9235")
}
