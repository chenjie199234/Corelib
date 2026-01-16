package cotel

import (
	"bytes"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
)

var lker sync.RWMutex

var last uint64
var cputype string
var cpunum float64
var curcpu float64 //percent

var memtype string
var totalmem uint64
var curmem uint64

func init() {
	cgroupv := ""
	if host.Container {
		if _, e := os.Lstat("/sys/fs/cgroup/cgroup.controllers"); e != nil {
			if !os.IsNotExist(e) {
				slog.Error("[cotel.system] cgroup version file failed",
					slog.String("file", "/sys/fs/cgroup/cgroup.controllers"),
					slog.String("error", e.Error()))
				return
			}
			cgroupv = "v1"
		} else {
			cgroupv = "v2"
		}
	}
	readcpu(cgroupv)
	readmem(cgroupv)
	go func() {
		tker := time.NewTicker(time.Millisecond * 500)
		for {
			<-tker.C
			lker.Lock()
			readcpu(cgroupv)
			readmem(cgroupv)
			lker.Unlock()
		}
	}()
}
func readcpu(cgroupv string) {
	cputype = ""
	cpunum = 0
	curcpu = 0
	if host.Container {
		if cgroupv == "v2" {
			if cm, e := os.ReadFile("/sys/fs/cgroup/cpu.max"); e != nil {
				if !os.IsNotExist(e) {
					slog.Error("[cotel.system.cpu] read file failed",
						slog.String("file", "/sys/fs/cgroup/cpu.max"),
						slog.String("error", e.Error()))
					return
				}
				cputype = "host"
			} else {
				cm = bytes.TrimSpace(cm)
				if bytes.HasPrefix(cm, []byte{'m', 'a', 'x'}) {
					cputype = "host"
				} else if parts := bytes.Fields(cm); len(parts) != 2 {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu.max"),
						slog.String("data", common.BTS(cm)))
				} else if limit, e := strconv.ParseUint(common.BTS(parts[0]), 10, 64); e != nil {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu.max"),
						slog.String("data", common.BTS(cm)))
				} else if limit == 0 {
					cpunum = 0
					cputype = "cgroupv2"
				} else if period, e := strconv.ParseUint(common.BTS(parts[1]), 10, 64); e != nil || period == 0 {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu.max"),
						slog.String("data", common.BTS(cm)))
				} else {
					cpunum = float64(limit) / float64(period)
					cputype = "cgroupv2"
				}
			}
		} else if cm, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"); e != nil {
			if !os.IsNotExist(e) {
				slog.Error("[cotel.system.cpu] read file failed",
					slog.String("file", "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"),
					slog.String("error", e.Error()))
				return
			}
			cputype = "host"
		} else {
			cm = bytes.TrimSpace(cm)
			if limit, e := strconv.ParseInt(common.BTS(cm), 10, 64); e != nil {
				slog.Error("[cotel.system.cpu] file data broken",
					slog.String("file", "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"),
					slog.String("data", common.BTS(cm)))
			} else if limit < 0 {
				cputype = "host"
			} else if limit == 0 {
				cpunum = 0
				cputype = "cgroupv1"
			} else if cp, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us"); e != nil {
				slog.Error("[cotel.system.cpu] read file failed",
					slog.String("file", "/sys/fs/cgroup/cpu/cpu.cfs_period_us"),
					slog.String("error", e.Error()))
			} else {
				cp = bytes.TrimSpace(cp)
				if period, e := strconv.ParseUint(common.BTS(cp), 10, 64); e != nil || period == 0 {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu/cpu.cfs_period_us"),
						slog.String("data", common.BTS(cp)))
				} else {
					cpunum = float64(uint64(limit)) / float64(period)
					cputype = "cgroupv1"
				}
			}
		}
	} else {
		cputype = "host"
	}
	switch cputype {
	case "cgroupv2":
		if cpunum == 0 {
			curcpu = 0
		} else if cs, e := os.ReadFile("/sys/fs/cgroup/cpu.stat"); e != nil {
			slog.Error("[cotel.system.cpu] read file failed",
				slog.String("file", "/sys/fs/cgroup/cpu.stat"),
				slog.String("error", e.Error()))
		} else {
			for line := range bytes.SplitSeq(cs, []byte{'\n'}) {
				line = bytes.TrimSpace(line)
				if !bytes.HasPrefix(line, common.STB("usage_usec")) {
					continue
				}
				parts := bytes.Fields(line)
				if len(parts) != 2 {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu.stat"),
						slog.String("data", common.BTS(line)))
				} else if now, e := strconv.ParseUint(common.BTS(parts[1]), 10, 64); e != nil {
					slog.Error("[cotel.system.cpu] file data broken",
						slog.String("file", "/sys/fs/cgroup/cpu.stat"),
						slog.String("data", common.BTS(parts[1])))
				} else {
					//the cpu time's unit in cgroupv2 is Microsecond
					tmp := float64(now-last) / (cpunum * 500_000) /*to Microsecond*/ * 100.0 /*to percent*/
					curcpu = math.Min(100, math.Max(0, tmp))
					last = now
				}
				break
			}
		}
	case "cgroupv1":
		if cpunum == 0 {
			curcpu = 0
		} else if s, e := os.ReadFile("/sys/fs/cgroup/cpu/cpuacct.usage"); e != nil {
			slog.Error("[cotel.system.cpu] read file failed",
				slog.String("file", "/sys/fs/cgroup/cpu/cpuacct.usage"),
				slog.String("error", e.Error()))
		} else {
			s = bytes.TrimSpace(s)
			if now, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
				slog.Error("[cotel.system.cpu] file data broken",
					slog.String("file", "/sys/fs/cgroup/cpu/cpuacct.usage"),
					slog.String("data", common.BTS(s)))
			} else {
				//the cpu time's unit in cgroupv1 is Nanosecond
				tmp := float64(now-last) / (cpunum * 500_000_000) /*to nanosecond*/ * 100.0 /*to percent*/
				curcpu = math.Min(100, math.Max(0, tmp))
				last = now
			}
		}
	case "host":
		cpunum = float64(runtime.NumCPU())
		if p, e := cpu.Percent(0, false); e != nil {
			slog.Error("[cotel.system.cpu] gopsutil get host cpu info failed", slog.String("error", e.Error()))
		} else {
			curcpu = p[0]
		}
	}
}
func readmem(cgroupv string) {
	memtype = ""
	totalmem = 0
	curmem = 0
	if host.Container {
		if cgroupv == "v2" {
			if mm, e := os.ReadFile("/sys/fs/cgroup/memory.max"); e != nil {
				if !os.IsNotExist(e) {
					slog.Error("[cotel.system.mem] read file failed",
						slog.String("file", "/sys/fs/cgroup/memory.max"),
						slog.String("error", e.Error()))
					return
				}
				memtype = "host"
			} else {
				mm = bytes.TrimSpace(mm)
				if bytes.HasPrefix(mm, []byte{'m', 'a', 'x'}) {
					memtype = "host"
				} else if limit, e := strconv.ParseUint(common.BTS(mm), 10, 64); e != nil {
					slog.Error("[cotel.system.mem] file data broken",
						slog.String("file", "/sys/fs/cgroup/memory.max"),
						slog.String("data", common.BTS(mm)))
				} else {
					totalmem = limit
					memtype = "cgroupv2"
				}
			}
		} else if mm, e := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); e != nil {
			if !os.IsNotExist(e) {
				slog.Error("[cotel.system.mem] read file failed",
					slog.String("file", "/sys/fs/cgroup/memory/memory.limit_in_bytes"),
					slog.String("error", e.Error()))
				return
			}
			memtype = "host"
		} else {
			mm = bytes.TrimSpace(mm)
			limit, e := strconv.ParseUint(common.BTS(mm), 10, 64)
			if e != nil {
				slog.Error("[cotel.system.mem] file data broken",
					slog.String("file", "/sys/fs/cgroup/memory/memory.limit_in_bytes"),
					slog.String("data", common.BTS(mm)))
			} else if limit == 9223372036854771712 || limit > 100000*1024*1024*1024 {
				//the 9223372036854771712 should be fixed,means no limit
				//add another check,if bigger then 100000G also means no limit,this is used to prevent the fixed magic number changed
				memtype = "host"
			} else {
				totalmem = limit
				memtype = "cgroupv1"
			}
		}
	} else {
		memtype = "host"
	}
	switch memtype {
	case "cgroupv2":
		if totalmem == 0 {
			return
		}
		if s, e := os.ReadFile("/sys/fs/cgroup/memory.current"); e != nil {
			slog.Error("[cotel.system.mem] read file failed",
				slog.String("file", "/sys/fs/cgroup/memory.current"),
				slog.String("error", e.Error()))
		} else {
			s = bytes.TrimSpace(s)
			if cur, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
				slog.Error("[cotel.system.mem] file data broken",
					slog.String("file", "/sys/fs/cgroup/memory.current"),
					slog.String("data", common.BTS(s)))
			} else {
				curmem = cur
			}
		}
	case "cgroupv1":
		if totalmem == 0 {
			return
		}
		if s, e := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); e != nil {
			slog.Error("[cotel.system.mem] read file failed",
				slog.String("file", "/sys/fs/cgroup/memory/memory.usage_in_bytes"),
				slog.String("error", e.Error()))
		} else {
			s = bytes.TrimSpace(s)
			if cur, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
				slog.Error("[cotel.system.mem] file data broken",
					slog.String("file", "/sys/fs/cgroup/memory/memory.usage_in_bytes"),
					slog.String("data", common.BTS(s)))
			} else {
				curmem = cur
			}
		}
	case "host":
		if info, e := mem.VirtualMemory(); e != nil {
			slog.Error("[cotel.system.mem] gopsutil get host memory info failed", slog.String("error", e.Error()))
		} else {
			totalmem = info.Total
			curmem = info.Used
		}
	}
}

func GetCpuMemUsage() (float64, float64, string, uint64, float64, string) {
	lker.RLock()
	defer lker.RUnlock()
	if totalmem == 0 {
		return cpunum, curcpu, cputype, 0, 0, memtype
	}
	return cpunum, curcpu, cputype, totalmem, float64(curmem) / float64(totalmem) * 100.0 /*to percent*/, memtype
}
