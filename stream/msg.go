package stream

import (
	"encoding/binary"

	"google.golang.org/protobuf/proto"
)

func makeHeartMsg(sender string, timestamp int64, starttime int64, needprefix bool) []byte {
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
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func makeVerifyMsg(sender string, verifydata []byte, starttime int64, needprefix bool) []byte {
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
	if needprefix {
		return addPrefix(data)
	}
	return data

}
func makeUserMsg(userdata []byte, starttime int64, needprefix bool) []byte {
	data, _ := proto.Marshal(&TotalMsg{
		Totaltype: TotalMsgType_USER,
		Starttime: starttime,
		Msg: &TotalMsg_User{
			User: &UserMsg{
				Userdata: userdata,
			},
		},
	})
	if needprefix {
		return addPrefix(data)
	}
	return data
}
func addPrefix(data []byte) []byte {
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(len(data)))
	return append(prefix, data...)
}
