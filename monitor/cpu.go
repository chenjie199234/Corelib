package monitor

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

var cpulker sync.Mutex

// how many logic cpu we can use
var CPUNum float64

var cgroupCPU bool

var MaxUsageCPU float64
var AverageUsageCPU float64
var LastUsageCPU float64

// physic
var cpuTotalStart int64
var cpuTotalLast int64
var cpuIdleStart int64
var cpuIdleLast int64

// cgroup
var cpuUsageStart int64
var cpuUsageStartTime int64
var cpuUsageLast int64
var cpuUsageLastTime int64

func initcpu() {
	if runtime.GOOS != "linux" {
		return
	}
	getCPUNum()
	if CPUNum == 0 {
		return
	}
	if cgroupCPU {
		cpuMetricCGROUP(time.Now().UnixNano())
	} else {
		cpuMetricPHYSIC()
	}
	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for {
			t := <-ticker.C
			cpulker.Lock()
			if cgroupCPU {
				cpuMetricCGROUP(t.UnixNano())
			} else {
				cpuMetricPHYSIC()
			}
			cpulker.Unlock()
		}
	}()
}

func cpuCollect() (float64, float64, float64) {
	cpulker.Lock()
	defer func() {
		MaxUsageCPU = 0
		cpulker.Unlock()
	}()
	if cgroupCPU {
		cpuUsageStart = cpuUsageLast
		cpuUsageStartTime = cpuUsageLastTime
	} else {
		cpuTotalStart = cpuTotalLast
		cpuIdleStart = cpuIdleLast
	}
	return LastUsageCPU, MaxUsageCPU, AverageUsageCPU
}

func getCPUNum() {
	quotastr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if e != nil && !os.IsNotExist(e) {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us:", e)
		return
	}
	if e == nil {
		if quotastr[len(quotastr)-1] == 10 {
			quotastr = quotastr[:len(quotastr)-1]
		}
		quota, e := strconv.ParseInt(common.Byte2str(quotastr), 10, 64)
		if e != nil {
			log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us data:", quotastr, "format wrong")
			return
		}
		if quota > 0 {
			cpunumCGROUP(quota)
			cgroupCPU = true
			return
		}
	}
	cpunumPHYSIC()
}

func cpunumCGROUP(quota int64) {
	periodstr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_period_us:", e)
		return
	}
	if periodstr[len(periodstr)-1] == 10 {
		periodstr = periodstr[:len(periodstr)-1]
	}
	period, e := strconv.ParseInt(common.Byte2str(periodstr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_period_us data:", periodstr, "format wrong")
		return
	}
	CPUNum = float64(quota) / float64(period)
}

func cpunumPHYSIC() {
	CPUNum = float64(runtime.NumCPU())
}

func cpuMetricCGROUP(now int64) {
	usagestr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpuacct.usage")
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage:", e)
		return
	}
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseInt(common.Byte2str(usagestr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage data:", usagestr, "format wrong")
		return
	}
	oldUsage := cpuUsageLast
	cpuUsageLast = usage
	cpuUsageLastTime = now
	if cpuUsageStart == 0 {
		cpuUsageStart = cpuUsageLast
		cpuUsageStartTime = now
		return
	}
	LastUsageCPU = float64(cpuUsageLast-oldUsage) / CPUNum / float64((time.Millisecond * 100).Nanoseconds())
	if LastUsageCPU > MaxUsageCPU {
		MaxUsageCPU = LastUsageCPU
	}
	AverageUsageCPU = float64(cpuUsageLast-cpuUsageStart) / CPUNum / float64(cpuUsageLastTime-cpuUsageStartTime)
}

func cpuMetricPHYSIC() {
	tmpdata, e := os.ReadFile("/proc/stat")
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /proc/stat:", e)
		return
	}
	var line string
	for _, line = range strings.Split(common.Byte2str(tmpdata), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		for i, c := range line {
			if c == '0' || c == '1' || c == '2' || c == '3' || c == '4' || c == '5' || c == '6' || c == '7' || c == '8' || c == '9' {
				line = line[i:]
				break
			}
		}
		break
	}
	pieces := strings.Split(line, " ")
	tmpTotal := int64(0)
	tmpIdle := int64(0)
	for i, piece := range pieces {
		if len(piece) == 0 {
			continue
		}
		value, e := strconv.ParseInt(piece, 10, 64)
		if e != nil {
			log.Error(nil, "[monitor.cpu] /proc/stat data broken:", e)
			return
		}
		tmpTotal += value
		if i == 3 || i == 4 {
			//idle or iowait
			tmpIdle += value
		}
	}
	if cpuTotalStart == 0 {
		cpuTotalLast = tmpTotal
		cpuIdleLast = tmpIdle
		cpuTotalStart = tmpTotal
		cpuIdleStart = tmpIdle
		return
	}
	LastUsageCPU = 1 - float64(tmpIdle-cpuIdleLast)/float64(tmpTotal-cpuTotalLast)
	if LastUsageCPU > MaxUsageCPU {
		MaxUsageCPU = LastUsageCPU
	}
	AverageUsageCPU = 1 - float64(tmpIdle-cpuIdleStart)/float64(tmpTotal-cpuTotalStart)
	cpuTotalLast = tmpTotal
	cpuIdleLast = tmpIdle
}
