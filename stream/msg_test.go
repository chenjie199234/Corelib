package stream

import (
	"bytes"
	"fmt"
	"testing"
)

func Test_Msg(t *testing.T) {
	testheartmsg()
	testverifymsg()
	testusermsg()
}

func testheartmsg() {
	data := makePingMsg([]byte("ping"))
	msgtype, e := getMsgType(data.Bytes()[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != PING {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
}
func testverifymsg() {
	data := makeVerifyMsg("test", []byte{'t', 'e', 's', 't'}, 1654)
	msgtype, e := getMsgType(data.Bytes()[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != VERIFY {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
	sender, verifydata, starttime, e := getVerifyMsg(data.Bytes()[4:])
	if e != nil {
		panic("get verify msg error:" + e.Error())
	}
	if sender != "test" || !bytes.Equal(verifydata, []byte{'t', 'e', 's', 't'}) || starttime != 1654 {
		panic("get verify msg error:data wrong")
	}
}
func testusermsg() {
	data := makeUserMsg([]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g'}, 1654)
	msgtype, e := getMsgType(data.Bytes()[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != USER {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
	temp, starttime, e := getUserMsg(data.Bytes()[4:])
	if e != nil {
		panic("get user msg error:" + e.Error())
	}
	if !bytes.Equal(temp, []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g'}) || starttime != 1654 {
		panic("get user msg error:data wrong")
	}
}
