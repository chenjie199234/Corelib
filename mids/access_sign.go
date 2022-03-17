package mids

import (
	"crypto/sha256"
	"encoding/hex"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/util/common"
)

type accesssign struct {
	seckeys map[string]string //key path,value seckey
}

var accesssignInstance *accesssign

func init() {
	accesssignInstance = &accesssign{
		seckeys: make(map[string]string),
	}
}
func UpdateAccessSignConfig(seckeys map[string]string) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&accesssignInstance.seckeys)), unsafe.Pointer(&seckeys))
}
func AccessSign(path, signdata, sign string) bool {
	seckeys := *(*map[string]string)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&accesssignInstance.seckeys))))
	seckey, ok := seckeys[path]
	if !ok {
		seckey, ok = seckeys["default"]
		if !ok {
			return false
		}
	}
	h := sha256.Sum256(common.Str2byte(signdata + seckey))
	return hex.EncodeToString(h[:]) == sign
}
