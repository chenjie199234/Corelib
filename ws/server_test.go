package ws

import (
	"bufio"
	"bytes"
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
			msgbuf := pool.GetPool().Get(0)
			ctlbuf := pool.GetPool().Get(0)
			for {
				opcode, e := Read(reader, &msgbuf, 1024, &ctlbuf, true)
				if e != nil {
					panic("read error:" + e.Error())
				}
				switch {
				case opcode.IsPing():
					fmt.Println(string(ctlbuf))
					if e := WritePong(conn, ctlbuf, false); e != nil {
						panic("write pong error:" + e.Error())
					}
					ctlbuf = ctlbuf[:0]
				case opcode.IsPong():
					fmt.Println(string(ctlbuf))
					ctlbuf = ctlbuf[:0]
				case opcode.IsClose():
					fmt.Println(string(ctlbuf))
					c.Close()
					ctlbuf = ctlbuf[:0]
					return
				default:
					fmt.Println("msglen:", len(msgbuf))
					if !bytes.Equal(msgbuf, bytes.Repeat([]byte("a"), 513)) {
						fmt.Println(string(msgbuf))
						panic("msg broken")
					}
					if e := WriteMsg(conn, msgbuf, true, true, false); e != nil {
						panic("write msg error:" + e.Error())
					}
					msgbuf = msgbuf[:0]
				}
			}
		}(conn)
	}
}
