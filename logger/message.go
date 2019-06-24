package logger

import (
	"encoding/binary"
	"github.com/gogo/protobuf/proto"
)

func makeHeartMsg() []byte {
	d, _ := proto.Marshal(&TotalMsg{
		Type: HEART,
	})
	return makeFinalMsg(d)
}

func makeVerifyMsg() []byte {
	d, _ := proto.Marshal(&TotalMsg{
		Type:   VERIFY,
		Verify: &VerifyMsg{},
	})
	return makeFinalMsg(d)
}
func makeLogMsg(serverid int64, log []byte, filename string, line uint32, memindex uint32) []byte {
	d, _ := proto.Marshal(&TotalMsg{
		Type: LOG,
		Log: &LogMsg{
			Serverid: serverid,
			Content:  log,
			Filename: filename,
			Line:     line,
			Memindex: memindex,
		},
	})
	return makeFinalMsg(d)
}
func makeConfirmMsg(filename string, line uint32, memindex uint32) []byte {
	d, _ := proto.Marshal(&TotalMsg{
		Type: CONFIRM,
		Confirm: &ConfirmMsg{
			Filename: filename,
			Line:     line,
			Memindex: memindex,
		},
	})
	return makeFinalMsg(d)
}
func makeRemoveMsg(year, month, day, hour int32) []byte {
	d, _ := proto.Marshal(&TotalMsg{
		Type: REMOVE,
		Remove: &RemoveMsg{
			Year:  year,
			Month: month,
			Day:   day,
			Hour:  hour,
		},
	})
	return makeFinalMsg(d)
}
func makeFinalMsg(data []byte) []byte {
	temp := make([]byte, 2)
	binary.BigEndian.PutUint16(temp, uint16(len(data)))
	return append(temp, data...)
}
func readFirstMsg(data *[]byte) (*TotalMsg, error) {
	if len(*data) < 2 {
		return nil, nil
	}
	head := binary.BigEndian.Uint16((*data)[:2])
	if len(*data) < int(head+2) {
		return nil, nil
	}
	var e error
	msg := new(TotalMsg)
	if head != 0 {
		e = proto.Unmarshal((*data)[2:2+head], msg)
	}
	*data = (*data)[2+head:]
	return msg, e
}
