package monitor

import (
	"runtime"
)

var LastGCWasteTime uint64

func golangCollect() (uint64, uint64, uint64) {
	routinenum := runtime.NumGoroutine()
	threadnum, _ := runtime.ThreadCreateProfile(nil)

	meminfo := &runtime.MemStats{}
	runtime.ReadMemStats(meminfo)
	gctime := meminfo.PauseTotalNs - uint64(LastGCWasteTime)
	LastGCWasteTime = meminfo.PauseTotalNs
	return uint64(routinenum), uint64(threadnum), gctime
}
