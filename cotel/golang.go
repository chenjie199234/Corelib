package cotel

import (
	"runtime"
)

var lastGCWasteTime uint64

func getGo() (uint64, uint64, uint64) {
	routinenum := runtime.NumGoroutine()
	threadnum, _ := runtime.ThreadCreateProfile(nil)

	meminfo := &runtime.MemStats{}
	runtime.ReadMemStats(meminfo)
	gctime := meminfo.PauseTotalNs - uint64(lastGCWasteTime)
	lastGCWasteTime = meminfo.PauseTotalNs
	return uint64(routinenum), uint64(threadnum), gctime
}
