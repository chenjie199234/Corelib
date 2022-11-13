package ws

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/pool"
)

func Test_Client(t *testing.T) {
	conn, e := net.Dial("tcp", "127.0.0.1:12345")
	if e != nil {
		t.Fatal("conn error:", e)
	}
	reader := bufio.NewReader(conn)
	if _, e = Cupgrade(reader, conn, "127.0.0.1", "/abc"); e != nil {
		t.Fatal("client upgrade error:", e)
	}
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		msgbuf := pool.GetBuffer()
		ctlbuf := pool.GetBuffer()
		for {
			opcode, e := Read(reader, msgbuf, 65535, ctlbuf, false)
			if e != nil {
				panic("read error:" + e.Error())
			}
			switch {
			case opcode.IsPing():
				fmt.Println(ctlbuf.String())
				if e := WritePong(conn, ctlbuf.Bytes(), true); e != nil {
					panic("write pong error:" + e.Error())
				}
				ctlbuf.Reset()
			case opcode.IsPong():
				fmt.Println(string(ctlbuf.Bytes()))
				ctlbuf.Reset()
			case opcode.IsClose():
				ctlbuf.Reset()
				conn.Close()
				return
			default:
				fmt.Println(string(msgbuf.Bytes()))
				msgbuf.Reset()
			}
		}
	}()
	go func() {
		for {
			time.Sleep(time.Second)
			data := []byte("123456789abcdefg")
			for len(data) > 0 {
				if e := WriteMsg(conn, data[:1], len(data) == 1, len(data) == 9, true); e != nil {
					panic("write msg error:" + e.Error())
				}
				if e := WritePing(conn, []byte("client ping"), true); e != nil {
					panic("write ping error:" + e.Error())
				}
				data = data[1:]
			}
		}
	}()
	wg.Wait()
}
