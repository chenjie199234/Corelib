package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func finder() {
	UpdateDiscoveryServers([]string{"server1:127.0.0.1:9234", "server2:127.0.0.1:9235"})
}
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
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500 * time.Millisecond,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'}, finder)
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
	time.Sleep(time.Second)
	RegisterSelf(&RegMsg{
		RpcIp:     "",
		RpcPort:   9000,
		WebIp:     "",
		WebPort:   8000,
		WebScheme: "https",
	})
	time.Sleep(time.Second * 10)
	for i := 0; i < 10; i++ {
		fmt.Println()
	}
	fmt.Println("unregister start")
	UnRegisterSelf()
	fmt.Println("unregister end")
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
		HeartbeatTimeout:   5 * time.Second,
		HeartprobeInterval: 2 * time.Second,
		GroupNum:           1,
		TcpC: &stream.TcpConfig{
			ConnectTimeout:       500 * time.Millisecond,
			SocketReadBufferLen:  1024,
			SocketWriteBufferLen: 1024,
			AppWriteBufferNum:    256,
		},
	}, []byte{'t', 'e', 's', 't'}, finder)
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
	time.Sleep(time.Second)
	RegisterSelf(&RegMsg{
		RpcIp:     "",
		RpcPort:   9001,
		WebIp:     "",
		WebPort:   8001,
		WebScheme: "https",
	})
	time.Sleep(time.Second * 10)
	for i := 0; i < 10; i++ {
		fmt.Println()
	}
	fmt.Println("unregister start")
	UnRegisterSelf()
	fmt.Println("unregister end")
	select {}
}
