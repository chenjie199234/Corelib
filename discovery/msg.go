package discovery

import (
	"bytes"

	"github.com/chenjie199234/Corelib/util/common"
)

const (
	msgonline  = 'a'
	msgoffline = 'b'
	msgpull    = 'c'
	split      = '|'
)

type RegMsg struct {
	WebScheme string `json:"hs,omitempty"`
	WebIp     string `json:"hi,omitempty"`
	WebPort   int    `json:"hp,omitempty"`
	RpcIp     string `json:"ri,omitempty"`
	RpcPort   int    `json:"rp,omitempty"`
	Addition  []byte `json:"a,omitempty"`
}

func makeOnlineMsg(appuniquename string, data []byte) []byte {
	result := make([]byte, len(appuniquename)+len(data)+2)
	result[0] = msgonline
	copy(result[1:len(appuniquename)+1], appuniquename)
	result[len(appuniquename)+1] = split
	copy(result[len(appuniquename)+2:len(appuniquename)+2+len(data)], data)
	return result
}
func getOnlineMsg(data []byte) (string, []byte) {
	if len(data) <= 1 {
		return "", nil
	}
	firstindex := bytes.Index(data, []byte{split})
	if firstindex == -1 || firstindex == 0 || firstindex == 1 || firstindex == (len(data)-1) {
		return "", nil
	}
	return common.Byte2str(data[1:firstindex]), data[firstindex+1:]
}
func makeOfflineMsg(peeruniquename string) []byte {
	result := make([]byte, len(peeruniquename)+1)
	result[0] = msgoffline
	copy(result[1:len(peeruniquename)+1], peeruniquename)
	return result
}
func getOfflineMsg(data []byte) string {
	if len(data) <= 1 {
		return ""
	}
	return common.Byte2str(data[1:])
}
func makePullMsg(appname string) []byte {
	result := make([]byte, 1+len(appname))
	result[0] = msgpull
	copy(result[1:], appname)
	return result
}
func getPullMsg(data []byte) string {
	if len(data) <= 1 {
		return ""
	}
	return common.Byte2str(data[1:])
}
