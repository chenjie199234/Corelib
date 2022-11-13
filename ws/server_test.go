package ws

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/pool"
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
			msgbuf := pool.GetBuffer()
			ctlbuf := pool.GetBuffer()
			for {
				opcode, e := Read(reader, msgbuf, 100, ctlbuf, true)
				if e != nil {
					panic("read error:" + e.Error())
				}
				switch {
				case opcode.IsPing():
					fmt.Println(ctlbuf.String())
					if e := WritePong(conn, ctlbuf.Bytes(), false); e != nil {
						panic("write pong error:" + e.Error())
					}
					ctlbuf.Reset()
				case opcode.IsPong():
					fmt.Println(string(ctlbuf.Bytes()))
					ctlbuf.Reset()
				case opcode.IsClose():
					fmt.Println(string(ctlbuf.Bytes()))
					c.Close()
					ctlbuf.Reset()
					return
				default:
					fmt.Println(string(msgbuf.Bytes()))
					if e := WriteMsg(conn, msgbuf.Bytes(), true, true, false); e != nil {
						panic("write msg error:" + e.Error())
					}
					msgbuf.Reset()
				}
			}
		}(conn)
	}
}
