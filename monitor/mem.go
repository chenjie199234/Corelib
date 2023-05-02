package monitor

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

var memlker sync.Mutex

var TotalMEM int64 //bytes

var cgroupMEM bool

var MaxUsageMEM float64
var LastUsageMEM float64

func initmem() {
	cgroupTotalstr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes:", e)
		return
	}
	if cgroupTotalstr[len(cgroupTotalstr)-1] == 10 {
		cgroupTotalstr = cgroupTotalstr[:len(cgroupTotalstr)-1]
	}
	cgrouptotal, e := strconv.ParseInt(common.Byte2str(cgroupTotalstr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.limit_in_bytes data:", cgroupTotalstr, "format wrong")
		return
	}
	physicstr, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		log.Error(nil, "[monitor.mem] read /proc/meminfo:", e)
		return
	}
	var physictotal int64
	for _, line := range strings.Split(common.Byte2str(physicstr), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		line = strings.TrimSpace(strings.Split(line, ":")[1])
		line = line[:len(line)-3]
		physictotal, e = strconv.ParseInt(line, 10, 64)
		if e != nil {
			log.Error(nil, "[monitor.mem] read /proc/meminfo data:", physicstr, "format wrong")
			return
		}
		physictotal *= 1024
		break
	}
	if cgrouptotal > physictotal {
		TotalMEM = physictotal
	} else {
		TotalMEM = cgrouptotal
		cgroupMEM = true
	}
	go func() {
		tker := time.NewTicker(time.Millisecond * 100)
		for {
			<-tker.C
			memlker.Lock()
			if cgroupMEM {
				memMetricCGROUP()
			} else {
				memMetricPHYSIC()
			}
			memlker.Unlock()
		}
	}()
}

func memCollect() (float64, float64) {
	memlker.Lock()
	defer func() {
		MaxUsageMEM = 0
		memlker.Unlock()
	}()
	return LastUsageMEM, MaxUsageMEM
}
func memMetricCGROUP() {
	usagestr, e := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes:", e)
		return
	}
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseInt(common.Byte2str(usagestr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.mem] read /sys/fs/cgroup/memory/memory.usage_in_bytes data:", usagestr, "format wrong")
		return
	}
	LastUsageMEM = float64(usage) / float64(TotalMEM)
	if LastUsageMEM > MaxUsageMEM {
		MaxUsageMEM = LastUsageMEM
	}
}
func memMetricPHYSIC() {
	physicstr, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		log.Error(nil, "[monitor.mem] read /proc/meminfo:", e)
		return
	}
	for _, line := range strings.Split(common.Byte2str(physicstr), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		line = strings.TrimSpace(strings.Split(line, ":")[1])
		line = line[:len(line)-3]
		physicavailable, e := strconv.ParseInt(line, 10, 64)
		if e != nil {
			log.Error(nil, "[monitor.mem] read /proc/meminfo data:", physicstr, "format wrong")
			return
		}
		LastUsageMEM = float64(physicavailable) / float64(TotalMEM)
		if LastUsageMEM > MaxUsageMEM {
			MaxUsageMEM = LastUsageMEM
		}
		break
	}
}
