package monitor

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/shirou/gopsutil/v3/cpu"
)

var cpulker sync.Mutex

var CPUNum float64
var MaxUsageCPU float64
var AverageUsageCPU float64
var LastUsageCPU float64

func initcpu() {
	cgroup := getCPUNum()
	if cgroup {
		cgroupCPU(time.Now().UnixNano())
	} else {
		gopsutilCPU()
	}
	go func() {
		ticker := time.NewTicker(time.Millisecond * 100)
		for {
			t := <-ticker.C
			cpulker.Lock()
			if cgroup {
				cgroupCPU(t.UnixNano())
			} else {
				gopsutilCPU()
			}
			cpulker.Unlock()
		}
	}()
}

func cpuCollect() (float64, float64, float64) {
	cpulker.Lock()
	defer func() {
		MaxUsageCPU = -MaxUsageCPU
		cpulker.Unlock()
	}()
	//cgroup
	cpuUsageStart = cpuUsageLast
	cpuUsageStartTime = cpuUsageLastTime

	//gopsutil
	cpuTotalStart = cpuTotalLast
	cpuIdleStart = cpuIdleLast
	if MaxUsageCPU < 0 {
		return LastUsageCPU, -MaxUsageCPU, AverageUsageCPU
	}
	return LastUsageCPU, MaxUsageCPU, AverageUsageCPU
}

func GetCPU() (float64, float64, float64) {
	cpulker.Lock()
	defer cpulker.Unlock()
	if MaxUsageCPU < 0 {
		return LastUsageCPU, -MaxUsageCPU, AverageUsageCPU
	}
	return LastUsageCPU, MaxUsageCPU, AverageUsageCPU
}

func getCPUNum() (cgroup bool) {
	defer func() {
		if CPUNum == 0 {
			CPUNum = float64(runtime.NumCPU())
		}
	}()
	if runtime.GOOS != "linux" {
		return false
	}
	quotastr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if e != nil {
		if !os.IsNotExist(e) {
			panic("[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us error: " + e.Error())
		}
		return false
	}
	//delete '/n' if exist
	if quotastr[len(quotastr)-1] == 10 {
		quotastr = quotastr[:len(quotastr)-1]
	}
	quota, e := strconv.ParseInt(common.BTS(quotastr), 10, 64)
	if e != nil {
		panic("[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us data format wrong: " + e.Error())
	}
	if quota <= 0 {
		return false
	}
	periodstr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if e != nil {
		panic("[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us success,but read /sys/fs/cgroup/cpu/cpu.cfs_period_us error: " + e.Error())
	}
	//delete '/n' if exist
	if periodstr[len(periodstr)-1] == 10 {
		periodstr = periodstr[:len(periodstr)-1]
	}
	period, e := strconv.ParseInt(common.BTS(periodstr), 10, 64)
	if e != nil {
		panic("[monitor.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us success,but read /sys/fs/cgroup/cpu/cpu.cfs_period_us data format wrong: " + e.Error())
	}
	CPUNum = float64(quota) / float64(period)
	return true
}

// gopsutil
var cpuTotalStart float64
var cpuTotalLast float64
var cpuIdleStart float64
var cpuIdleLast float64

func gopsutilCPU() {
	times, _ := cpu.Times(false)
	total := times[0].User + times[0].System + times[0].Idle + times[0].Nice + times[0].Iowait + times[0].Irq + times[0].Softirq + times[0].Steal + times[0].Guest + times[0].GuestNice
	idle := times[0].Idle + times[0].Iowait
	if cpuTotalStart == 0 {
		cpuTotalLast = total
		cpuTotalStart = total
		cpuIdleLast = idle
		cpuIdleStart = idle
		return
	}
	LastUsageCPU = 1 - (idle-cpuIdleLast)/(total-cpuTotalLast)
	if LastUsageCPU > MaxUsageCPU {
		MaxUsageCPU = LastUsageCPU
	}
	AverageUsageCPU = 1 - (idle-cpuIdleStart)/(total-cpuTotalStart)
	cpuTotalLast = total
	cpuIdleLast = idle
}

// cgroup
var cpuUsageStart int64
var cpuUsageStartTime int64
var cpuUsageLast int64
var cpuUsageLastTime int64

// now: timestamp(nanosecond)
func cgroupCPU(now int64) {
	usagestr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpuacct.usage")
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage failed", log.CError(e))
		return
	}
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseInt(common.BTS(usagestr), 10, 64)
	if e != nil {
		log.Error(nil, "[monitor.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage data format wrong", log.String("usage", common.BTS(usagestr)))
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
