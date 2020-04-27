package stream

import (
	"encoding/binary"

	"google.golang.org/protobuf/proto"
)

func makeHeartMsg(sender string, timestamp int64, starttime int64) []byte {
	data, _ := proto.Marshal(&TotalMsg{
		Totaltype: TotalMsgType_HEART,
		Sender:    sender,
		Starttime: starttime,
		Msg: &TotalMsg_Heart{
			Heart: &HeartMsg{
				Timestamp: timestamp,
			},
		},
	})
	return addPrefix(data)
}
func makeVerifyMsg(sender string, verifydata []byte, starttime int64) []byte {
	data, _ := proto.Marshal(&TotalMsg{
		Totaltype: TotalMsgType_VERIFY,
		Sender:    sender,
		Starttime: starttime,
		Msg: &TotalMsg_Verify{
			Verify: &VerifyMsg{
				Verifydata: verifydata,
			},
		},
	})
	return addPrefix(data)
}
func makeUserMsg(sender string, userdata []byte, starttime int64) []byte {
	data, _ := proto.Marshal(&TotalMsg{
		Totaltype: TotalMsgType_USER,
		Sender:    sender,
		Starttime: starttime,
		Msg: &TotalMsg_User{
			User: &UserMsg{
				Userdata: userdata,
			},
		},
	})
	return addPrefix(data)
}
func addPrefix(data []byte) []byte {
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(len(data)))
	return append(prefix, data...)
}
