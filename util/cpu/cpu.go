package cpu

import (
	"math"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/shirou/gopsutil/cpu"
)

var usepercent float64

func init() {
	info, e := cpu.Percent(0, false)
	if e != nil {
		log.Error("[Sys.cpu]Get cpu use percent error:", e)
	} else if len(info) > 0 {
		atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
	} else {
		log.Error("[Sys.cpu]Get cpu use percent return empty")
	}
	time.Sleep(100 * time.Millisecond)
	info, e = cpu.Percent(0, false)
	if e != nil {
		log.Error("[Sys.cpu]Get cpu use percent error:", e)
	} else if len(info) > 0 {
		atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
	} else {
		log.Error("[Sys.cpu]Get cpu use percent return empty")
	}
	tker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			<-tker.C
			info, e := cpu.Percent(0, false)
			if e != nil {
				log.Error("[Sys.cpu]Get cpu use percent error:", e)
			} else if len(info) > 0 {
				atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
			} else {
				log.Error("[Sys.cpu]Get cpu use percent return empty")
			}
		}
	}()
}
func GetUse() float64 {
	temp := math.Float64frombits(atomic.LoadUint64((*uint64)(unsafe.Pointer(&usepercent))))
	if temp < 1 {
		return 1
	}
	return temp
}
