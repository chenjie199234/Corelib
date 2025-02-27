package cotel

import (
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/shirou/gopsutil/v4/mem"
)

var memlker sync.RWMutex

var totalMem uint64     //bytes
var maxUsageMEM uint64  //bytes
var lastUsageMEM uint64 //bytes

func init() {
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

func collectMEM() (uint64, uint64, uint64) {
	memlker.Lock()
	defer func() {
		maxUsageMEM = -maxUsageMEM
		memlker.Unlock()
	}()
	if maxUsageMEM < 0 {
		return totalMem, lastUsageMEM, -maxUsageMEM
	}
	return totalMem, lastUsageMEM, maxUsageMEM
}

func GetMEM() (uint64, uint64, uint64) {
	memlker.RLock()
	defer memlker.RUnlock()
	if maxUsageMEM < 0 {
		return totalMem, lastUsageMEM, -maxUsageMEM
	}
	return totalMem, lastUsageMEM, maxUsageMEM
}

func getTotalMEM() (cgroup bool) {
	defer func() {
		if totalMem == 0 {
			memory, _ := mem.VirtualMemory()
			totalMem = memory.Total
		}
	}()
	if runtime.GOOS != "linux" {
		return false
	}
	limitstr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if e != nil {
		if !os.IsNotExist(e) {
			panic("[cotel.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes error: " + e.Error())
		}
		return false
	}
	//drop '/n' if exist
	if limitstr[len(limitstr)-1] == 10 {
		limitstr = limitstr[:len(limitstr)-1]
	}
	limit, e := strconv.ParseUint(common.BTS(limitstr), 10, 64)
	if e != nil {
		panic("[cotel.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes data format wrong:" + e.Error())
	}
	memory, e := mem.VirtualMemory()
	if e != nil {
		panic("[cotel.mem] get pc memory info error: " + e.Error())
	}
	if memory.Total > limit && limit != 0 {
		totalMem = limit
		return true
	}
	totalMem = memory.Total
	return false
}

func cgroupMEM() {
	usagestr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if e != nil {
		slog.Error("[cotel.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes failed", slog.String("error", e.Error()))
		return
	}
	//drop \n
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseUint(common.BTS(usagestr), 10, 64)
	if e != nil {
		slog.Error("[cotel.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes data format wrong", slog.String("usage_in_bytes", common.BTS(usagestr)))
		return
	}
	lastUsageMEM = usage
	if lastUsageMEM > maxUsageMEM {
		maxUsageMEM = lastUsageMEM
	}
}
func gopsutilMEM() {
	memory, _ := mem.VirtualMemory()
	lastUsageMEM = memory.Used
	if lastUsageMEM > maxUsageMEM {
		maxUsageMEM = lastUsageMEM
	}
}
