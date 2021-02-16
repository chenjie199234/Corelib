package memory

import (
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/shirou/gopsutil/mem"
)

var usepercent float64

func init() {
	meminfo, e := mem.VirtualMemory()
	if e != nil {
		log.Error("[Sys.Memory]Get memory use percent error:", e)
	} else {
		usepercent = meminfo.UsedPercent
	}
	tker := time.NewTicker(time.Millisecond * 200)
	go func() {
		for {
			<-tker.C
			meminfo, e := mem.VirtualMemory()
			if e != nil {
				log.Error("[Sys.Memory]Get memory use percent error:", e)
			} else {
				usepercent = meminfo.UsedPercent
			}
		}
	}()
}
func GetUse() float64 {
	return usepercent
}
