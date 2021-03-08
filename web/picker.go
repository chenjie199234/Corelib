package web

import (
	"math"
	"math/rand"
	"time"
)

func defaultPicker(servers []*ServerForPick) *ServerForPick {
	if len(servers) == 0 {
		return nil
	}
	if len(servers) == 1 {
		return servers[0]
	}
	start := rand.Intn(len(servers))
	i := start
	first := true
	var normal1, normal2, danger1, danger2 *ServerForPick
	before := time.Now().Add(-time.Millisecond * 100)
	for {
		if !first && i == start {
			break
		}
		first = false
		if servers[i].Pickinfo.DServers != 0 &&
			servers[i].Pickinfo.DServerOffline < before.UnixNano() &&
			servers[i].Pickinfo.Lastfail < before.UnixNano() {
			if normal1 == nil {
				normal1 = servers[i]
			} else {
				normal2 = servers[i]
				break
			}
		} else {
			if danger1 == nil {
				danger1 = servers[i]
			} else if danger2 == nil {
				danger2 = servers[i]
			}
		}
		i++
		if i >= len(servers) {
			i = 0
		}
	}
	if normal1 != nil && normal2 == nil {
		return normal1
	} else if normal2 != nil && normal1 == nil {
		return normal2
	} else if normal1 == nil && normal2 == nil {
		if danger1 != nil && danger2 == nil {
			return danger1
		} else if danger2 != nil && danger1 == nil {
			return danger2
		} else {
			normal1 = danger1
			normal2 = danger2
		}
	}
	//more discoveryservers more safety,so 1 * 2's discoveryserver num
	load1 := float64(normal1.Pickinfo.Activecalls) * math.Log(float64(normal2.Pickinfo.DServers+2))
	//more discoveryservers more safety,so 2 * 1's discoveryserver num
	load2 := float64(normal2.Pickinfo.Activecalls) * math.Log(float64(normal1.Pickinfo.DServers+2))
	if load1 > load2 {
		return normal2
	} else if load1 < load2 {
		return normal1
	} else if rand.Intn(2) == 0 {
		return normal1
	} else {
		return normal2
	}
}
