package memory

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/mem"
)

var usepercent float64

func init() {
	tker := time.NewTicker(time.Millisecond * 200)
	go func() {
		for {
			<-tker.C
			meminfo, e := mem.VirtualMemory()
			if e != nil {
				fmt.Printf("[Sys.Memory]Get memory use percent error:%s\n", e)
			} else {
				usepercent = meminfo.UsedPercent
			}
		}
	}()
}
func GetUse() float64 {
	return usepercent
}
