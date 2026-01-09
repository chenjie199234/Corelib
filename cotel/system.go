package cotel

import (
	"bytes"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
)

var last uint64
var cpunum float64
var curcpu float64 //percent

var totalmem uint64
var curmem uint64

var cputype string
var memtype string

func init() {
	if host.Container {
		cgroupversion := ""
		if _, e := os.Lstat("/sys/fs/cgroup/cgroup.controllers"); e != nil {
			if !os.IsNotExist(e) {
				panic("check /sys/fs/cgroup/cgroup.controllers exist or not failed,error:" + e.Error())
			}
			cgroupversion = "v1"
		} else {
			cgroupversion = "v2"
		}
		if cgroupversion == "v2" {
			if cm, e := os.ReadFile("/sys/fs/cgroup/cpu.max"); e != nil {
				if !os.IsNotExist(e) {
					panic("read /sys/fs/cgroup/cpu.max failed,error:" + e.Error())
				}
				cputype = "host"
			} else {
				cm = bytes.TrimSpace(cm)
				if bytes.HasPrefix(cm, []byte{'m', 'a', 'x'}) {
					cputype = "host"
				} else {
					parts := bytes.Fields(cm)
					if len(parts) != 2 {
						panic("/sys/fs/cgroup/cpu.max data broken")
					}
					limit, e := strconv.ParseUint(common.BTS(parts[0]), 10, 64)
					if e != nil {
						panic("/sys/fs/cgroup/cpu.max data broken")
					}
					period, e := strconv.ParseUint(common.BTS(parts[1]), 10, 64)
					if e != nil {
						panic("/sys/fs/cgroup/cpu.max data broken")
					}
					cpunum = float64(limit) / float64(period)
					cputype = "cgroupv2"
				}
			}
			if mm, e := os.ReadFile("/sys/fs/cgroup/memory.max"); e != nil {
				if !os.IsNotExist(e) {
					panic("read /sys/fs/cgroup/memory.max failed,error:" + e.Error())
				}
				memtype = "host"
			} else if bytes.HasPrefix(mm, []byte{'m', 'a', 'x'}) {
				memtype = "host"
			} else {
				mm = bytes.TrimSpace(mm)
				limit, e := strconv.ParseUint(common.BTS(mm), 10, 64)
				if e != nil {
					panic("/sys/fs/cgroup/memory.max data broken")
				}
				totalmem = limit
				memtype = "cgroupv2"
			}
		} else {
			if cm, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us"); e != nil {
				if !os.IsNotExist(e) {
					panic("read /sys/fs/cgroup/cpu/cpu.cfs_quota_us failed,error:" + e.Error())
				}
				cputype = "host"
			} else {
				cm = bytes.TrimSpace(cm)
				limit, e := strconv.ParseInt(common.BTS(cm), 10, 64)
				if e != nil {
					panic("/sys/fs/cgroup/cpu/cpu.cfs_quota_us data broken")
				}
				if limit < 0 {
					cputype = "host"
				} else {
					cp, e := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
					if e != nil {
						panic("read /sys/fs/cgroup/cpu/cpu.cfs_period_us failed,error:" + e.Error())
					}
					cp = bytes.TrimSpace(cp)
					period, e := strconv.ParseUint(common.BTS(cp), 10, 64)
					if e != nil {
						panic("/sys/fs/cgroup/cpu/cpu.cfs_period_us data broken")
					}
					cpunum = float64(uint64(limit)) / float64(period)
					cputype = "cgroupv1"
				}
			}
			if mm, e := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); e != nil {
				if !os.IsNotExist(e) {
					panic("read /sys/fs/cgroup/memory/memory.limit_in_bytes failed,error:" + e.Error())
				}
				memtype = "host"
			} else {
				mm = bytes.TrimSpace(mm)
				limit, e := strconv.ParseUint(common.BTS(mm), 10, 64)
				if e != nil {
					panic("/sys/fs/cgroup/memory/memory.limit_in_bytes data broken")
				}
				//the 9223372036854771712 should be fixed,means no limit
				//add another check,if bigger then 100000G also means no limit,this is used to prevent the fixed magic number changed
				if limit == 9223372036854771712 || limit > 100000*1024*1024*1024 {
					memtype = "host"
				} else {
					totalmem = limit
					memtype = "cgroupv1"
				}
			}
		}
	} else {
		cputype = "host"
		memtype = "host"
	}
	if cputype == "host" {
		cpunum = float64(runtime.NumCPU())
	}
	if memtype == "host" {
		info, e := mem.VirtualMemory()
		if e != nil {
			panic("gopsutil get host memory info failed,error:" + e.Error())
		}
		totalmem = info.Total
		curmem = info.Used
	}
	go func() {
		tker := time.NewTicker(time.Millisecond * 500)
		for {
			<-tker.C
			switch cputype {
			case "cgroupv2":
				if cs, e := os.ReadFile("/sys/fs/cgroup/cpu.stat"); e != nil {
					slog.Error("[cotel.system] read /sys/fs/cgroup/cpu.stat failed", slog.String("error", e.Error()))
				} else {
					for line := range bytes.SplitSeq(cs, []byte{'\n'}) {
						line = bytes.TrimSpace(line)
						if !bytes.HasPrefix(line, common.STB("usage_usec")) {
							continue
						}
						parts := bytes.Fields(line)
						if len(parts) != 2 {
							slog.Error("[cotel.system] /sys/fs/cgroup/cpu.stat data broken")
						}
						now, e := strconv.ParseUint(common.BTS(parts[1]), 10, 64)
						if e != nil {
							slog.Error("[cotel.system] /sys/fs/cgroup/cpu.stat data broken",
								slog.String("data", common.BTS(parts[1])), slog.String("error", e.Error()))
						}
						//the cpu time's unit in cgroupv2 is Microsecond
						tmp := (float64(now-last) * 1000.0) /*to nanosecond*/ / (cpunum * 500_000_000) /*to nanosecond*/ * 100.0 /*to percent*/
						curcpu = math.Min(100, math.Max(0, tmp))
						last = now
						break
					}
				}
			case "cgroupv1":
				s, e := os.ReadFile("/sys/fs/cgroup/cpu/cpuacct.usage")
				if e != nil {
					slog.Error("[cotel.system] read /sys/fs/cgroup/cpu/cpuacct.usage", slog.String("error", e.Error()))
				} else {
					s = bytes.TrimSpace(s)
					if now, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
						slog.Error("[cotel.system] /sys/fs/cgroup/cpu/cpuacct.usage data broken",
							slog.String("data", common.BTS(s)), slog.String("error", e.Error()))
					} else {
						//the cpu time's unit in cgroupv1 is Nanosecond
						tmp := float64(now-last) / (cpunum * 500_000_000) /*to nanosecond*/ * 100.0 /*to percent*/
						curcpu = math.Min(100, math.Max(0, tmp))
						last = now
					}
				}
			case "host":
				if p, e := cpu.Percent(0, false); e != nil {
					slog.Error("[cotel.system] gopsutil get host cpu info failed", slog.String("error", e.Error()))
				} else {
					curcpu = p[0]
				}
			}
			switch memtype {
			case "cgroupv2":
				s, e := os.ReadFile("/sys/fs/cgroup/memory.current")
				if e != nil {
					slog.Error("[cotel.system] read /sys/fs/cgroup/memory.current failed", slog.String("error", e.Error()))
				} else {
					s = bytes.TrimSpace(s)
					if cur, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
						slog.Error("[cotel.system] data in /sys/fs/cgroup/memory.current broken",
							slog.String("data", common.BTS(s)), slog.String("error", e.Error()))
					} else {
						curmem = cur
					}
				}
			case "cgroupv1":
				s, e := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
				if e != nil {
					slog.Error("[cotel.system] read /sys/fs/cgroup/memory/memory.usage_in_bytes failed", slog.String("error", e.Error()))
				} else {
					s = bytes.TrimSpace(s)
					if cur, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
						slog.Error("[cotel.system] data in /sys/fs/cgroup/memory/memory.usage_in_bytes broken",
							slog.String("data", common.BTS(s)), slog.String("error", e.Error()))
					} else {
						curmem = cur
					}
				}
			case "host":
				if info, e := mem.VirtualMemory(); e != nil {
					slog.Error("[cotel.system] gopsutil get host memory info failed", slog.String("error", e.Error()))
				} else {
					curmem = info.Used
				}
			}
		}
	}()
}

func GetCpuMemUsage() (float64, float64, string, uint64, float64, string) {
	return cpunum, curcpu, cputype, totalmem, float64(curmem) / float64(totalmem) * 100.0 /*to percent*/, memtype
}
