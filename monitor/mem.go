package monitor

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/shirou/gopsutil/v3/mem"
)

var memlker sync.Mutex

var TotalMem uint64     //bytes
var MaxUsageMEM uint64  //bytes
var LastUsageMEM uint64 //bytes

func initmem() {
	cgroup := getTotalMEM()
	go func() {
		tker := time.NewTicker(time.Millisecond * 100)
		for {
			<-tker.C
			memlker.Lock()
			if cgroup {
				cgroupMEM()
			} else {
				gopsutilMEM()
			}
			memlker.Unlock()
		}
	}()
}

func memCollect() (uint64, uint64, uint64) {
	memlker.Lock()
	defer func() {
		MaxUsageMEM = -MaxUsageMEM
		memlker.Unlock()
	}()
	if MaxUsageMEM < 0 {
		return TotalMem, LastUsageMEM, -MaxUsageMEM
	}
	return TotalMem, LastUsageMEM, MaxUsageMEM
}

func GetMEM() (uint64, uint64, uint64) {
	memlker.Lock()
	defer memlker.Unlock()
	if MaxUsageMEM < 0 {
		return TotalMem, LastUsageMEM, -MaxUsageMEM
	}
	return TotalMem, LastUsageMEM, MaxUsageMEM
}

func getTotalMEM() (cgroup bool) {
	defer func() {
		if TotalMem == 0 {
			memory, _ := mem.VirtualMemory()
			TotalMem = memory.Total
		}
	}()
	if runtime.GOOS != "linux" {
		return false
	}
	limitstr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if e != nil {
		if !os.IsNotExist(e) {
			panic("[monitor.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes error: " + e.Error())
		}
		return false
	}
	//drop '/n' if exist
	if limitstr[len(limitstr)-1] == 10 {
		limitstr = limitstr[:len(limitstr)-1]
	}
	limit, e := strconv.ParseUint(common.BTS(limitstr), 10, 64)
	if e != nil {
		panic("[monitor.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes data format wrong:" + e.Error())
	}
	memory, e := mem.VirtualMemory()
	if e != nil {
		panic("[monitor.mem] get pc memory info error: " + e.Error())
	}
	if memory.Total > limit && limit != 0 {
		TotalMem = limit
		return true
	}
	TotalMem = memory.Total
	return false
}

func cgroupMEM() {
	usagestr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes failed", log.CError(e))
		return
	}
	//drop \n
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseUint(common.BTS(usagestr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes data format wrong", log.String("usage_in_bytes", common.BTS(usagestr)))
		return
	}
	LastUsageMEM = usage
	if LastUsageMEM > MaxUsageMEM {
		MaxUsageMEM = LastUsageMEM
	}
}
func gopsutilMEM() {
	memory, _ := mem.VirtualMemory()
	LastUsageMEM = memory.Used
	if LastUsageMEM > MaxUsageMEM {
		MaxUsageMEM = LastUsageMEM
	}
}
