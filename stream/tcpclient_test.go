package stream

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

var clientinstance *Instance
var wg *sync.WaitGroup

func Test_Tcpclient(t *testing.T) {
	wg = new(sync.WaitGroup)
	clientinstance = NewInstance(&Config{
		SelfName:      "test2",
		VerifyTimeout: 250,
		VerifyData:    []byte{},
		HeartInterval: 500,
	}, client_HandleVerifyFunc, client_HandleOnlineFunc, client_HandleUserdataFunc, client_HandleOfflineFunc)
	clientinstance.StartTcpClient("127.0.0.1:9234")
	wg.Wait()
}

func client_HandleOnlineFunc(p *Peer, uniqueid int64) {
	fmt.Println("online")
	wg.Add(1)
	go func() {
		for {
			time.Sleep(time.Second)
			clientinstance.SendMessage(p, []byte("hello world!"), uniqueid)
		}
	}()
}
func client_HandleVerifyFunc(selfVerifyData, peerVerifyData []byte, peername string) bool {
	return true
}
func client_HandleUserdataFunc(p *Peer, uniqueid int64, data []byte) {
	fmt.Printf("%s\n", data)
}
func client_HandleOfflineFunc(p *Peer, uniqueid int64) {
	wg.Done()
	fmt.Println("offline")
}
