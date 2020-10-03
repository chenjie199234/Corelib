package test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/stream"
)

func Test_Discoveryserver(t *testing.T) {
	go func() {
		http.HandleFunc("/discoveryservers", func(w http.ResponseWriter, r *http.Request) {
			d, _ := json.Marshal([]string{"discoveryserver:127.0.0.1:9234"})
			w.WriteHeader(200)
			w.Write(d)
			return
		})
		http.ListenAndServe("127.0.0.1:8080", nil)
	}()
	discovery.NewDiscoveryServer(&stream.InstanceConfig{
		SelfName:           "discoveryserver",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		GroupNum:           1,
	}, []byte{'t', 'e', 's', 't'})
	discovery.StartDiscoveryServer(&stream.TcpConfig{
		ConnectTimeout:       500,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, "127.0.0.1:9234")
}
