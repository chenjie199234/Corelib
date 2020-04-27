package stream

import (
	"fmt"
	"testing"
)

var serverinstance *Instance

func Test_Tcpserver(t *testing.T) {
	serverinstance = NewInstance(&Config{
		SelfName:      "test1",
		VerifyTimeout: 250,
		VerifyData:    []byte{},
		HeartInterval: 500,
	}, server_HandleVerifyFunc, server_HandleOnlineFunc, server_HandleUserdataFunc, server_HandleOfflineFunc)
	serverinstance.StartTcpServer("127.0.0.1:9234")
	select {}
}

func server_HandleOnlineFunc(p *Peer, uniqueid int64) {
	fmt.Println("online")
}
func server_HandleVerifyFunc(selfVerifyData, peerVerifyData []byte, peername string) bool {
	return true
}
func server_HandleUserdataFunc(p *Peer, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
	serverinstance.SendMessage(p, data, uniqueid)
}
func server_HandleOfflineFunc(p *Peer, uniqueid int64) {
	fmt.Println("offline")
}
