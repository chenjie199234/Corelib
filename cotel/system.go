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

var cputype string = "cgroup"
var memtype string = "cgroup"

func init() {
	if host.Container {
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
			}
		}
		if mm, e := os.ReadFile("/sys/fs/cgroup/memory.max"); e != nil {
			if !os.IsNotExist(e) {
				panic("read /sys/fs/cgroup/memory.max failed,error:" + e.Error())
			}
			memtype = "host"
		} else if bytes.Equal(mm, []byte{'m', 'a', 'x'}) {
			memtype = "host"
		} else {
			mm = bytes.TrimSpace(mm)
			limit, e := strconv.ParseUint(common.BTS(mm), 10, 64)
			if e != nil {
				panic("/sys/fs/cgroup/memory.max data broken")
			}
			totalmem = limit
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
			if cputype == "cgroup" {
				if cs, e := os.ReadFile("/sys/fs/cgroup/cpu.stat"); e != nil {
					slog.Error("read /sys/fs/cgroup/cpu.stat failed", slog.String("error", e.Error()))
				} else {
					for line := range bytes.SplitSeq(cs, []byte{'\n'}) {
						line = bytes.TrimSpace(line)
						if !bytes.HasPrefix(line, common.STB("usage_usec")) {
							continue
						}
						parts := bytes.Fields(line)
						if len(parts) != 2 {
							slog.Error("/sys/fs/cgroup/cpu.stat data broken")
						}
						now, e := strconv.ParseUint(common.BTS(parts[1]), 10, 64)
						if e != nil {
							slog.Error("/sys/fs/cgroup/cpu.stat data broken", slog.String("error", e.Error()))
						}
						tmp := (float64(now-last) * 1000.0) /*to nanosecond*/ / (cpunum * 500_000_000) /*to nanosecond*/ * 100.0 /*to percent*/
						curcpu = math.Min(100, math.Max(0, tmp))
						last = now
						break
					}
				}
			} else {
				if p, e := cpu.Percent(0, false); e != nil {
					slog.Error("gopsutil get host cpu info failed", slog.String("error", e.Error()))
				} else {
					curcpu = p[0]
				}
			}
			if memtype == "cgroup" {
				s, e := os.ReadFile("/sys/fs/cgroup/memory.current")
				if e != nil {
					slog.Error("read /sys/fs/cgroup/memory.current failed", slog.String("error", e.Error()))
				} else {
					s = bytes.TrimSpace(s)
					if cur, e := strconv.ParseUint(common.BTS(s), 10, 64); e != nil {
						slog.Error("data in /sys/fs/cgroup/memory.current broken",
							slog.String("data", common.BTS(s)), slog.String("error", e.Error()))
					} else {
						curmem = cur
					}
				}
			} else {
				if info, e := mem.VirtualMemory(); e != nil {
					slog.Error("gopsutil get host memory info failed", slog.String("error", e.Error()))
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
