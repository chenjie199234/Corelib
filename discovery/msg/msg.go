package msg

import (
	"google.golang.org/protobuf/proto"
)

func MakeOnlineMsg(peer string, hash []byte) []byte {
	t := &Totalmsg{}
	t.Mtype = MSGTYPE_ONLINE
	t.Specialmsg = &Totalmsg_Online{
		Online: &Onlinemsg{
			Name:    peer,
			Newhash: hash,
		},
	}
	data, _ := proto.Marshal(t)
	return data
}
func MakeOfflineMsg(peer string, hash []byte) []byte {
	t := &Totalmsg{}
	t.Mtype = MSGTYPE_OFFLINE
	t.Specialmsg = &Totalmsg_Offline{
		Offline: &Offlinemsg{
			Name:    peer,
			Newhash: hash,
		},
	}
	data, _ := proto.Marshal(t)
	return data
}
func MakePullMsg() []byte {
	t := &Totalmsg{}
	t.Mtype = MSGTYPE_PULL
	t.Specialmsg = &Totalmsg_Pull{
		Pull: &Pullmsg{},
	}
	data, _ := proto.Marshal(t)
	return data
}
func MakePushMsg(peers []string) []byte {
	t := &Totalmsg{}
	t.Mtype = MSGTYPE_PUSH
	t.Specialmsg = &Totalmsg_Push{
		Push: &Pushmsg{
			Addrs: peers,
		},
	}
	data, _ := proto.Marshal(t)
	return data
}
