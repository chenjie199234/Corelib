package stream

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func Test_Msg(t *testing.T) {
	testheartmsg()
	testusermsg()
}
func testheartmsg() {
	heartmsg := &heartMsg{
		uniqueid: uint64(time.Now().UnixNano()),
		sender:   "test",
	}
	heartmsg.timestamp = heartmsg.uniqueid
	data := makeHeartMsg(heartmsg, true)
	msgtype, e := getMsgType(data[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != HEART {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
	temp, e := getHeartMsg(data[4:])
	if e != nil {
		panic("get heart msg error:" + e.Error())
	}
	if temp.uniqueid != heartmsg.uniqueid || temp.sender != heartmsg.sender || temp.timestamp != heartmsg.timestamp {
		panic("get heart msg error:data wrong")
	}
}
func testverifymsg() {
	verifymsg := &verifyMsg{
		uniqueid:   uint64(time.Now().UnixNano()),
		sender:     "test",
		verifydata: []byte{'t', 'e', 's', 't'},
	}
	data := makeVerifyMsg(verifymsg, true)
	msgtype, e := getMsgType(data[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != VERIFY {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
	temp, e := getVerifyMsg(data[4:])
	if e != nil {
		panic("get verify msg error:" + e.Error())
	}
	if temp.uniqueid != verifymsg.uniqueid || temp.sender != verifymsg.sender || !bytes.Equal(temp.verifydata, verifymsg.verifydata) {
		panic("get verify msg error:data wrong")
	}
}
func testusermsg() {
	usermsg := &userMsg{
		uniqueid: uint64(time.Now().UnixNano()),
		sender:   "test",
		userdata: []byte("abcdefg"),
	}
	data := makeUserMsg(usermsg, true)
	msgtype, e := getMsgType(data[4:])
	if e != nil {
		panic("get msg type error:" + e.Error())
	}
	if msgtype != USER {
		panic(fmt.Sprintf("get msg type error:type:%d wrong", msgtype))
	}
	temp, e := getUserMsg(data[4:])
	if e != nil {
		panic("get user msg error:" + e.Error())
	}
	if temp.uniqueid != usermsg.uniqueid || temp.sender != usermsg.sender || !bytes.Equal(temp.userdata, usermsg.userdata) {
		panic("get user msg error:data wrong")
	}
}
