package ws

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"
)

func Test_Server(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:12345")
	if e != nil {
		t.Fatal("listen error:", e)
	}
	for {
		conn, e := l.Accept()
		if e != nil {
			t.Fatal("accept error:", e)
		}
		go func(c net.Conn) {
			reader := bufio.NewReader(c)
			path, header, e := Supgrade(reader, conn)
			if e != nil {
				panic("server upgrade error:" + e.Error())
			}
			t.Log(path)
			t.Log(header)
			go func() {
				for {
					time.Sleep(time.Second)
					WritePing(c, []byte("server ping"), false)
				}
			}()
			if e := Read(reader, 1024, true, func(opcode OPCode, data []byte) bool {
				switch {
				case opcode.IsPing():
					fmt.Println(string(data))
					if e := WritePong(conn, data, false); e != nil {
						panic("write pong error:" + e.Error())
					}
				case opcode.IsPong():
					fmt.Println("client pong:" + string(data))
				case opcode.IsClose():
					fmt.Println(string(data))
					c.Close()
					return false
				default:
					fmt.Println("msglen:", len(data))
					if !bytes.Equal(data, bytes.Repeat([]byte("a"), 513)) {
						fmt.Println(string(data))
						panic("msg broken")
					}
					if e := WriteMsg(conn, data, true, true, false); e != nil {
						panic("write msg error:" + e.Error())
					}
				}
				return true
			}); e != nil {
				t.Error(e)
			}
		}(conn)
	}
}
