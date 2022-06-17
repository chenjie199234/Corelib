package mids

import (
	"sync/atomic"
	"unsafe"
)

type accesskey struct {
	seckeys map[string][]string //key path,value seckey
}

var accesskeyInstance *accesskey

func init() {
	accesskeyInstance = &accesskey{
		seckeys: make(map[string][]string),
	}
}

//seckeys's map key is path
//seckeys's map value is the seckey
func UpdateAccessKeyConfig(seckeys map[string][]string) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&accesskeyInstance.seckeys)), unsafe.Pointer(&seckeys))
}
func AccessKey(path string, key string) bool {
	seckeys := *(*map[string][]string)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&accesskeyInstance.seckeys))))
	seckey, ok := seckeys[path]
	if !ok {
		seckey, ok = seckeys["default"]
		if !ok {
			return false
		}
	}
	for _, v := range seckey {
		if v == key {
			return true
		}
	}
	return false
}
