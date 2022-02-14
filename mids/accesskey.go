package mids

import (
	"sync/atomic"
	"unsafe"
)

type access struct {
	kv map[string]string
}

var accessinstance *access

func init() {
	accessinstance = &access{
		kv: make(map[string]string),
	}
}

//key accessID,value accessKEY
func UpdateAccessConfig(kv map[string]string) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&accessinstance.kv)), unsafe.Pointer(&kv))
}

func Access(accessID, accessKEY string) bool {
	kv := *(*map[string]string)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&accessinstance.kv))))
	if key, ok := kv[accessID]; !ok || key != accessKEY {
		return false
	}
	return true
}
