package stream

//import (
//        "bytes"
//        "context"
//        "fmt"
//        "net/http"
//        _ "net/http/pprof"
//        "os"
//        "runtime"
//        "sync/atomic"
//        "testing"
//        "time"
//)

//var webserverinstance *Instance

//func Test_Webserver(t *testing.T) {
//        runtime.GOMAXPROCS(runtime.NumCPU())
//        webserverinstance = NewInstance(&InstanceConfig{
//                SelfName:           "server",
//                VerifyTimeout:      500,
//                HeartbeatTimeout:   1500,
//                HeartprobeInterval: 500,
//                RecvIdleTimeout:    30000, //30s
//                GroupNum:           10,
//                Verifyfunc:         webserverhandleVerify,
//                Onlinefunc:         webserverhandleonline,
//                Userdatafunc:       webserverhandleuserdata,
//                Offlinefunc:        webserverhandleoffline,
//        })
//        os.Remove("./test.socket")
//        go webserverinstance.StartWebsocketServer([]string{"/test"}, "127.0.0.1:9235", func(*http.Request) bool { return true })
//        go func() {
//                for {
//                        time.Sleep(time.Second)
//                        fmt.Println("client num:", webserverinstance.totalpeernum)
//                }
//        }()
//        http.ListenAndServe(":8084", nil)
//}
//func webserverhandleVerify(ctx context.Context, peeruniquename string, peerVerifyData []byte) ([]byte, bool) {
//        if !bytes.Equal([]byte{'t', 'e', 's', 't', 'c'}, peerVerifyData) {
//                fmt.Println("verify error")
//                return nil, false
//        }
//        return []byte{'t', 'e', 's', 't'}, true
//}
//func webserverhandleonline(p *Peer, peeruniquename string, starttime uint64) {
//}
//func webserverhandleuserdata(p *Peer, peeruniquename string, data []byte, starttime uint64) {
//        fmt.Printf("%s:%s\n", peeruniquename, data)
//        p.SendMessage(data, starttime, true)
//}
//func webserverhandleoffline(p *Peer, peeruniquename string, starttime uint64) {
//}
