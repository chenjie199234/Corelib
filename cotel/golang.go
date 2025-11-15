package cotel

import (
	"runtime"
	"runtime/debug"
)

func getGo() (uint64, uint64, uint64) {
	routinenum := runtime.NumGoroutine()
	threadnum, _ := runtime.ThreadCreateProfile(nil)
	gcinfo := &debug.GCStats{}
	debug.ReadGCStats(gcinfo)
	return uint64(routinenum), uint64(threadnum), uint64(gcinfo.PauseTotal.Nanoseconds())
}
