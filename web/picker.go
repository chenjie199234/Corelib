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
	var normal1 *ServerForPick
	var normal2 *ServerForPick
	var danger1 *ServerForPick
	var danger2 *ServerForPick
	var nightmare1 *ServerForPick
	var nightmare2 *ServerForPick
	now := time.Now()
	for {
		if !first && i == start {
			break
		}
		first = false
		onesbefore := now.Add(-time.Second)
		if servers[i].Pickinfo.DiscoveryServers != 0 &&
			servers[i].Pickinfo.DiscoveryServerOfflineTime < onesbefore.UnixNano() &&
			(servers[i].Pickinfo.Lastcall < onesbefore.UnixNano() || servers[i].Pickinfo.Cpu != 100) {
			if normal1 == nil {
				normal1 = servers[i]
			} else {
				normal2 = servers[i]
				break
			}
		} else if servers[i].Pickinfo.DiscoveryServers == 0 {
			if nightmare1 == nil {
				nightmare1 = servers[i]
			} else if nightmare2 == nil {
				nightmare1 = servers[i]
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
	//check normal
	if normal1 != nil && normal2 != nil {
	} else if normal1 != nil {
		return normal1
	} else if normal2 != nil {
		return normal2
	}
	//check danger
	if danger1 != nil && danger2 != nil {
		normal1 = danger1
		normal2 = danger2
	} else if danger1 != nil {
		return danger1
	} else if danger2 != nil {
		return danger2
	}
	//check nightmare
	if nightmare1 != nil && nightmare2 != nil {
		normal1 = nightmare1
		normal2 = nightmare2
	} else if nightmare1 != nil {
		return nightmare1
	} else if nightmare2 != nil {
		return nightmare2
	}
	//more discoveryservers more safety,so 1 * 2's discoveryserver num
	load1 := normal1.Pickinfo.Cpu * math.Log1p(float64(normal1.Pickinfo.Activecalls)) * math.Log1p(float64(normal2.Pickinfo.DiscoveryServers))
	//more discoveryservers more safety,so 2 * 1's discoveryserver num
	load2 := normal2.Pickinfo.Cpu * math.Log1p(float64(normal2.Pickinfo.Activecalls)) * math.Log1p(float64(normal1.Pickinfo.DiscoveryServers))
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
