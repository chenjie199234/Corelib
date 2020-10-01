package cpu

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

var usepercent float64

func init() {
	tker := time.NewTicker(200 * time.Millisecond)
	go func() {
		for {
			<-tker.C
			info, e := cpu.Percent(0, false)
			if e != nil {
				fmt.Printf("[Sys.cpu]Get cpu use percent error:%s\n", e)
			} else if len(info) > 0 {
				usepercent = info[0]
			} else {
				fmt.Printf("[Sys.cpu]Get cpu use percent return empty\n")
			}
		}
	}()
}
func GetUse() float64 {
	return usepercent
}
