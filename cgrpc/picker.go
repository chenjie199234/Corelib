package cgrpc

import (
	"math"
	"math/rand"
	"time"
)

func defaultPicker(servers []*ServerForPick) *ServerForPick {
	if len(servers) == 0 {
		return nil
	}
	var normal1, normal2, danger1, danger2, nightmare1, nightmare2 *ServerForPick
	before := time.Now().Add(-time.Millisecond * 100)
	startindex := rand.Intn(len(servers))
	endindex := startindex
	for {
		if server := servers[startindex]; server.Pickable() {
			if server.Pickinfo.DServerNum != 0 &&
				server.Pickinfo.DServerOffline < before.UnixNano() {
				if normal1 == nil {
					normal1 = server
				} else {
					normal2 = server
					break
				}
			} else if server.Pickinfo.DServerNum == 0 {
				if nightmare1 == nil {
					nightmare1 = server
				} else if nightmare2 == nil {
					nightmare2 = server
				}
			} else {
				if danger1 == nil {
					danger1 = server
				} else if danger2 == nil {
					danger2 = server
				}
			}
		}
		startindex++
		if startindex >= len(servers) {
			startindex = 0
		}
		if endindex == startindex {
			break
		}
	}
	//check normal
	if normal1 != nil && normal2 == nil {
		return normal1
	} else if normal2 != nil && normal1 == nil {
		return normal2
	} else if normal1 == nil && normal2 == nil {
		//check danger
		if danger1 != nil && danger2 == nil {
			return danger1
		} else if danger2 != nil && danger1 == nil {
			return danger2
		} else if danger1 != nil && danger2 != nil {
			normal1 = danger1
			normal2 = danger2
		} else if nightmare1 != nil && nightmare2 == nil {
			//check nightmare
			return nightmare1
		} else if nightmare2 != nil && nightmare1 == nil {
			return nightmare2
		} else if nightmare1 != nil && nightmare2 != nil {
			normal1 = nightmare1
			normal2 = nightmare2
		} else {
			//all servers are unpickable
			return nil
		}
	}
	//more discoveryservers more safety,so 1 * 2's dserver num
	load1 := float64(normal1.Pickinfo.Activecalls) * math.Log(float64(normal2.Pickinfo.DServerNum+2))
	if normal1.Pickinfo.LastFailTime >= before.UnixNano() {
		//punish
		load1 *= 1.1
	}
	//more discoveryservers more safety,so 2 * 1's dserver num
	load2 := float64(normal2.Pickinfo.Activecalls) * math.Log(float64(normal1.Pickinfo.DServerNum+2))
	if normal2.Pickinfo.LastFailTime >= before.UnixNano() {
		//punish
		load2 *= 1.1
	}
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
