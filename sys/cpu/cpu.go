package cpu

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/cpu"
)

var usepercent float64

func init() {
	info, e := cpu.Percent(0, false)
	if e != nil {
		fmt.Printf("[Sys.cpu]Get cpu use percent error:%s\n", e)
	} else if len(info) > 0 {
		atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
	} else {
		fmt.Printf("[Sys.cpu]Get cpu use percent return empty\n")
	}
	time.Sleep(100 * time.Millisecond)
	info, e = cpu.Percent(0, false)
	if e != nil {
		fmt.Printf("[Sys.cpu]Get cpu use percent error:%s\n", e)
	} else if len(info) > 0 {
		atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
	} else {
		fmt.Printf("[Sys.cpu]Get cpu use percent return empty\n")
	}
	tker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			<-tker.C
			info, e := cpu.Percent(0, false)
			if e != nil {
				fmt.Printf("[Sys.cpu]Get cpu use percent error:%s\n", e)
			} else if len(info) > 0 {
				atomic.StoreUint64((*uint64)(unsafe.Pointer(&usepercent)), math.Float64bits(info[0]))
			} else {
				fmt.Printf("[Sys.cpu]Get cpu use percent return empty\n")
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
