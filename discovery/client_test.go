package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Client1(t *testing.T) {
	//go func() {
	//        tker := time.NewTicker(time.Second)
	//        for {
	//                <-tker.C
	//                if clientinstance != nil {
	//                        clientinstance.lker.RLock()
	//                        if server, ok := clientinstance.servers["server1:127.0.0.1:9234"]; ok {
	//                                fmt.Println(server.allapps)
	//                        } else {
	//                                fmt.Println("server1: not online")
	//                        }
	//                        if server, ok := clientinstance.servers["server2:127.0.0.1:9235"]; ok {
	//                                fmt.Println(server.allapps)
	//                        } else {
	//                                fmt.Println("server2: not online")
	//                        }
	//                        clientinstance.lker.RUnlock()
	//                }
	//        }
	//}()
	NewDiscoveryClient(&stream.InstanceConfig{
		SelfName:           "client",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'}, "http://127.0.0.1:8080/discoveryservers")
	time.Sleep(time.Second)
	rch, e := NoticeRpcChanges("client")
	if e != nil {
		panic("notice grpc change error:" + e.Error())
	}
	wch, e := NoticeWebChanges("client")
	if e != nil {
		panic("notice http change error:" + e.Error())
	}
	go func() {
		for {
			select {
			case <-rch:
				r, addition := GetRpcInfos("client")
				fmt.Println(r)
				fmt.Printf("%s\n", addition)
			case <-wch:
				r, addition := GetWebInfos("client")
				fmt.Println(r)
				fmt.Printf("%s\n", addition)
			}
		}
	}()
	RegisterSelf(&RegMsg{
		RpcIp:     "",
		RpcPort:   9000,
		WebIp:     "",
		WebPort:   8000,
		WebScheme: "https",
	})
	select {}
}
func Test_Client2(t *testing.T) {
	//go func() {
	//        tker := time.NewTicker(time.Second)
	//        for {
	//                <-tker.C
	//                if clientinstance != nil {
	//                        clientinstance.lker.RLock()
	//                        if server, ok := clientinstance.servers["server1:127.0.0.1:9234"]; ok {
	//                                fmt.Println(server.allapps)
	//                        } else {
	//                                fmt.Println("server1: not online")
	//                        }
	//                        if server, ok := clientinstance.servers["server2:127.0.0.1:9235"]; ok {
	//                                fmt.Println(server.allapps)
	//                        } else {
	//                                fmt.Println("server2: not online")
	//                        }
	//                        clientinstance.lker.RUnlock()
	//                }
	//        }
	//}()
	NewDiscoveryClient(&stream.InstanceConfig{
		SelfName:           "client",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'}, "http://127.0.0.1:8080/discoveryservers")
	time.Sleep(time.Second)
	rch, e := NoticeRpcChanges("client")
	if e != nil {
		panic("notice grpc change error:" + e.Error())
	}
	wch, e := NoticeWebChanges("client")
	if e != nil {
		panic("notice http change error:" + e.Error())
	}
	go func() {
		for {
			select {
			case <-rch:
				r, addition := GetRpcInfos("client")
				fmt.Println(r)
				fmt.Printf("%s\n", addition)
			case <-wch:
				r, addition := GetWebInfos("client")
				fmt.Println(r)
				fmt.Printf("%s\n", addition)
			}
		}
	}()
	RegisterSelf(&RegMsg{
		RpcIp:     "",
		RpcPort:   9001,
		WebIp:     "",
		WebPort:   8001,
		WebScheme: "https",
	})
	select {}
}
