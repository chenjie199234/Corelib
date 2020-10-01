package discovery

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/stream"
)

func Test_Client1(t *testing.T) {
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			<-tker.C
			if clientinstance != nil {
				clientinstance.lker.RLock()
				if server, ok := clientinstance.servers["server1:127.0.0.1:9234"]; ok {
					fmt.Println("server1:" + hex.EncodeToString(server.htree.GetRootHash()))
				}
				if server, ok := clientinstance.servers["server2:127.0.0.1:9235"]; ok {
					fmt.Println("server2:" + hex.EncodeToString(server.htree.GetRootHash()))
				}
				clientinstance.lker.RUnlock()
			}
		}
	}()
	NewDiscoveryClient(&stream.InstanceConfig{
		SelfName:           "client",
		VerifyTimeout:      500,
		HeartbeatTimeout:   5000,
		HeartprobeInterval: 2000,
		GroupNum:           1,
	}, &stream.TcpConfig{
		ConnectTimeout:       500,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
		AppMinReadBufferLen:  1024,
		AppMaxReadBufferLen:  65535,
		AppWriteBufferNum:    256,
	}, []byte{'t', 'e', 's', 't'}, "http://127.0.0.1:8080/discoveryservers")
	time.Sleep(time.Second)
	gexists, gch, _ := GrpcNotice("client")
	fmt.Println(gexists)
	hexists, hch, _ := HttpNotice("client")
	fmt.Println(hexists)
	texists, tch, _ := TcpNotice("client")
	fmt.Println(texists)
	wexists, wch, _ := WebSocketNotice("client")
	fmt.Println(wexists)
	go func() {
		for {
			var noticemsg *NoticeMsg
			select {
			case noticemsg = <-gch:
				if noticemsg.PeerAddr != "127.0.0.1:9000" && noticemsg.PeerAddr != "0.0.0.0:9000" {
					panic("reg message broken")
				}
				if noticemsg.DiscoveryServer != "server1:127.0.0.1:9234" && noticemsg.DiscoveryServer != "server2:127.0.0.1:9235" {
					panic("reg message broken")
				}
			case noticemsg = <-hch:
				if noticemsg.PeerAddr != "127.0.0.1:8000" && noticemsg.PeerAddr != "0.0.0.0:8000" {
					panic("reg message broken")
				}
				if noticemsg.DiscoveryServer != "server1:127.0.0.1:9234" && noticemsg.DiscoveryServer != "server2:127.0.0.1:9235" {
					panic("reg message broken")
				}
			case noticemsg = <-tch:
				if noticemsg.PeerAddr != "127.0.0.1:7000" && noticemsg.PeerAddr != "0.0.0.0:7000" {
					panic("reg message broken")
				}
				if noticemsg.DiscoveryServer != "server1:127.0.0.1:9234" && noticemsg.DiscoveryServer != "server2:127.0.0.1:9235" {
					panic("reg message broken")
				}
			case noticemsg = <-wch:
				if noticemsg.PeerAddr != "127.0.0.1:6000" && noticemsg.PeerAddr != "0.0.0.0:6000" {
					panic("reg message broken")
				}
				if noticemsg.DiscoveryServer != "server1:127.0.0.1:9234" && noticemsg.DiscoveryServer != "server2:127.0.0.1:9235" {
					panic("reg message broken")
				}
			}
			fmt.Printf("get notice msg:%+v\n", noticemsg)
		}
	}()
	RegisterSelf(&RegMsg{
		GrpcIp:      "",
		GrpcPort:    9000,
		HttpIp:      "",
		HttpPort:    8000,
		TcpIp:       "",
		TcpPort:     7000,
		WebSockIp:   "",
		WebSockPort: 6000,
	})
	select {}
}
