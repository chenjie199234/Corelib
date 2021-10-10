package stream

import (
	"bytes"
	"testing"
)

func Test_Msg(t *testing.T) {
	testheartmsg()
	testverifymsg()
	testusermsg()
}

func testheartmsg() {
	data := makePingMsg([]byte("ping"))
	msgtype, pingdata, _, _, e := decodeMsg(data.Bytes()[4:])
	if e != nil || msgtype != PING || string(pingdata) != "ping" {
		panic("ping msg error")
	}
	data = makePongMsg([]byte("pong"))
	msgtype, pingdata, _, _, e = decodeMsg(data.Bytes()[4:])
	if e != nil || msgtype != PONG || string(pingdata) != "pong" {
		panic("pong msg error")
	}
}
func testverifymsg() {
	data := makeVerifyMsg("test", []byte{'t', 'e', 's', 't'}, 1654)
	msgtype, verifydata, sender, maxmsglength, e := decodeMsg(data.Bytes()[4:])
	if e != nil || msgtype != VERIFY || sender != "test" || !bytes.Equal(verifydata, []byte{'t', 'e', 's', 't'}) || maxmsglength != 10 {
		panic("verify msg error")
	}
}
func testusermsg() {
	data := makeUserMsg([]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g'})
	msgtype, userdata, _, _, e := decodeMsg(data.Bytes()[4:])
	if e != nil || msgtype != USER || !bytes.Equal(userdata, []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g'}) {
		panic("user msg error")
	}
}
