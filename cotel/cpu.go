package cotel

import (
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/shirou/gopsutil/v4/cpu"
)

var cpulker sync.RWMutex

var CPUNum float64
var maxUsageCPU float64
var averageUsageCPU float64
var lastUsageCPU float64

func init() {
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

func collectCPU() (float64, float64, float64) {
	cpulker.Lock()
	defer func() {
		maxUsageCPU = -maxUsageCPU
		cpulker.Unlock()
	}()
	//cgroup
	cpuUsageStart = cpuUsageLast
	cpuUsageStartTime = cpuUsageLastTime

	//gopsutil
	cpuTotalStart = cpuTotalLast
	cpuIdleStart = cpuIdleLast
	if maxUsageCPU < 0 {
		return lastUsageCPU, -maxUsageCPU, averageUsageCPU
	}
	return lastUsageCPU, maxUsageCPU, averageUsageCPU
}

func GetCPU() (float64, float64, float64) {
	cpulker.RLock()
	defer cpulker.RUnlock()
	if maxUsageCPU < 0 {
		return lastUsageCPU, -maxUsageCPU, averageUsageCPU
	}
	return lastUsageCPU, maxUsageCPU, averageUsageCPU
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
			panic("[cotel.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us error: " + e.Error())
		}
		return false
	}
	//delete '/n' if exist
	if quotastr[len(quotastr)-1] == 10 {
		quotastr = quotastr[:len(quotastr)-1]
	}
	quota, e := strconv.ParseInt(common.BTS(quotastr), 10, 64)
	if e != nil {
		panic("[cotel.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us data format wrong: " + e.Error())
	}
	if quota <= 0 {
		return false
	}
	periodstr, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if e != nil {
		panic("[cotel.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us success,but read /sys/fs/cgroup/cpu/cpu.cfs_period_us error: " + e.Error())
	}
	//delete '/n' if exist
	if periodstr[len(periodstr)-1] == 10 {
		periodstr = periodstr[:len(periodstr)-1]
	}
	period, e := strconv.ParseInt(common.BTS(periodstr), 10, 64)
	if e != nil {
		panic("[cotel.cpu] read /sys/fs/cgroup/cpu/cpu.cfs_quota_us success,but read /sys/fs/cgroup/cpu/cpu.cfs_period_us data format wrong: " + e.Error())
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
	lastUsageCPU = 1 - (idle-cpuIdleLast)/(total-cpuTotalLast)
	if lastUsageCPU > maxUsageCPU {
		maxUsageCPU = lastUsageCPU
	}
	averageUsageCPU = 1 - (idle-cpuIdleStart)/(total-cpuTotalStart)
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
		slog.Error("[cotel.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage failed", slog.String("error", e.Error()))
		return
	}
	if usagestr[len(usagestr)-1] == 10 {
		usagestr = usagestr[:len(usagestr)-1]
	}
	usage, e := strconv.ParseInt(common.BTS(usagestr), 10, 64)
	if e != nil {
		slog.Error("[cotel.cpu] read /sys/fs/cgroup/cpu/cpuacct.usage data format wrong", slog.String("usage", common.BTS(usagestr)))
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
	lastUsageCPU = float64(cpuUsageLast-oldUsage) / CPUNum / float64((time.Millisecond * 100).Nanoseconds())
	if lastUsageCPU > maxUsageCPU {
		maxUsageCPU = lastUsageCPU
	}
	averageUsageCPU = float64(cpuUsageLast-cpuUsageStart) / CPUNum / float64(cpuUsageLastTime-cpuUsageStartTime)
}
