package ws

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
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
		if e := Read(reader, 65535, false, func(opcode OPCode, data []byte) bool {
			switch {
			case opcode.IsPing():
				fmt.Println("server ping:" + string(data))
				if e := WritePong(conn, data, true); e != nil {
					panic("write pong error:" + e.Error())
				}
			case opcode.IsPong():
				fmt.Println("server pong:" + string(data))
			case opcode.IsClose():
				conn.Close()
				return false
			default:
				fmt.Println("msg len:", len(data))
				if !bytes.Equal(data, bytes.Repeat([]byte("a"), 513)) {
					fmt.Println(string(data))
					panic("msg broken")
				}
			}
			return true
		}); e != nil {
			t.Error(e)
		}
	}()
	go func() {
		for {
			time.Sleep(time.Second)
			data := bytes.Repeat([]byte("a"), 513)
			for len(data) > 0 {
				if e := WritePing(conn, []byte("client ping"), true); e != nil {
					panic("write ping error:" + e.Error())
				}
				var piece []byte
				if len(data) > 32 {
					piece = data[:32]
				} else {
					piece = data
				}
				if e := WriteMsg(conn, piece, len(data) <= 32, len(data) == 513, true); e != nil {
					panic("write msg error:" + e.Error())
				}
				if len(data) > 32 {
					data = data[32:]
				} else {
					data = nil
				}
			}
		}
	}()
	wg.Wait()
}
