package web

import (
	"math"
	"math/rand"
	"sync/atomic"
	"time"
)

func defaultPicker(servers map[string]*ServerForPick) *ServerForPick {
	if len(servers) == 0 {
		return nil
	}
	var normal1, normal2, danger1, danger2 *ServerForPick
	before := time.Now().Add(-time.Millisecond * 100)
	for _, server := range servers {
		if server.Pickable() {
			if server.Pickinfo.DServerNum != 0 &&
				server.Pickinfo.DServerOffline < before.UnixNano() {
				if normal1 == nil {
					normal1 = server
				} else {
					normal2 = server
					break
				}
			} else {
				if danger1 == nil {
					danger1 = server
				} else if danger2 == nil {
					danger2 = server
				}
			}
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
		} else if danger1 != nil && danger2 != nil {
			normal1 = danger1
			normal2 = danger2
		} else {
			//all servers are unpickable
			return nil
		}
	}
	//more discoveryservers more safety,so 1 * 2's discoveryserver num
	load1 := float64(atomic.LoadInt32(&normal1.Pickinfo.Activecalls)) + math.Log(float64(normal2.Pickinfo.DServerNum+2))
	if atomic.LoadInt64(&normal1.Pickinfo.Lastfail) >= before.UnixNano() {
		load1 *= 1.1
	}
	//more discoveryservers more safety,so 2 * 1's discoveryserver num
	load2 := float64(atomic.LoadInt32(&normal2.Pickinfo.Activecalls)) + math.Log(float64(normal1.Pickinfo.DServerNum+2))
	if atomic.LoadInt64(&normal2.Pickinfo.Lastfail) >= before.UnixNano() {
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
