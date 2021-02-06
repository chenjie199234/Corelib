package rpc

import (
	"math"
	"math/rand"
	"sync/atomic"
	"time"
	"unsafe"
)

var r *rand.Rand

func defaultPicker(servers []*Serverapp) *Serverapp {
	if r == nil {
		temp := rand.New(rand.NewSource(time.Now().UnixNano()))
		atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&r)), nil, unsafe.Pointer(temp))
	}
	if len(servers) == 1 {
		if servers[0].Pickable() {
			return servers[0]
		} else {
			return nil
		}
	}
	now := time.Now().Unix()
	var normala, normalb, dangera, dangerb, nightmarea, nightmareb *Serverapp
	start := r.Intn(len(servers))
	index := start
	for {
		if servers[index].Pickable() {
			switch {
			case servers[index].Pickinfo.DiscoveryServers == 0:
				//nigntmare
				if nightmarea == nil {
					nightmarea = servers[index]
				} else if nightmareb == nil {
					nightmareb = servers[index]
				}
			case now-servers[index].Pickinfo.DiscoveryServerOfflineTime <= 2:
				//danger
				if dangera == nil {
					dangera = servers[index]
				} else if dangerb == nil {
					dangerb = servers[index]
				}
			default:
				//normal
				if normala == nil {
					normala = servers[index]
				} else if normalb == nil {
					normalb = servers[index]
					break
				}
			}
		}
		index++
		if index == len(servers) {
			index = 0
		}
		if index == start {
			break
		}
	}
	if normala != nil && normalb != nil {

	} else if normala != nil {
		return normala
	} else if dangera != nil && dangerb != nil {
		normala = dangera
		normalb = dangerb
	} else if dangera != nil {
		return dangera
	} else if nightmarea != nil && nightmareb != nil {
		normala = nightmarea
		normalb = nightmareb
	} else if nightmarea != nil {
		return nightmarea
	} else {
		return nil
	}
	loada := math.Sqrt(float64(normala.Pickinfo.Netlag)) *
		normala.Pickinfo.Cpu *
		float64(normala.Pickinfo.Activecalls) *
		math.Log1p(float64(normalb.Pickinfo.DiscoveryServers)) //more discoveryservers more safety,so a * b's discoveryserver num
	loadb := math.Sqrt(float64(normalb.Pickinfo.Netlag)) *
		normalb.Pickinfo.Cpu *
		float64(normalb.Pickinfo.Activecalls) *
		math.Log1p(float64(normala.Pickinfo.DiscoveryServers)) //more discoveryservers more safety,so b * a's discoveryserver num
	if loada < loadb {
		return normala
	} else if loada > loadb {
		return normalb
	} else if rand.Intn(2) == 0 {
		return normala
	} else {
		return normalb
	}
}
