package mrpc

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

var defaultpickpool *sync.Pool
var r *rand.Rand

func getpickpool(length int) []int {
	r, ok := defaultpickpool.Get().([]int)
	if ok {
		return r[:0]
	} else {
		return make([]int, 0, length)
	}
}
func putpickpool(buf []int) {
	defaultpickpool.Put(buf)
}

func defaultPicker(servers []*Serverinfo) *Serverinfo {
	if len(servers) == 1 {
		if servers[0].Pickable() {
			return servers[0]
		} else {
			return nil
		}
	}
	normal := getpickpool(len(servers))
	danger := getpickpool(len(servers))
	nigntmare := getpickpool(len(servers))
	defer func() {
		putpickpool(normal)
		putpickpool(danger)
		putpickpool(nigntmare)
	}()
	now := time.Now().Unix()
	for i, server := range servers {
		if server.Pickable() {
			if server.Pickinfo.DiscoveryServers == 0 {
				nigntmare = append(nigntmare, i)
			} else if now-server.Pickinfo.DiscoveryServerOfflineTime <= 2 {
				danger = append(danger, i)
			} else {
				normal = append(normal, i)
			}
		}
	}
	var sa *Serverinfo
	var sb *Serverinfo
	if len(normal) == 1 {
		return servers[normal[0]]
	} else if len(normal) == 2 {
		sa = servers[normal[0]]
		sb = servers[normal[1]]
	} else if len(normal) > 2 {
		//rand pick two
		a := r.Intn(len(normal))
		b := r.Intn(len(normal) - 1)
		if b >= a {
			b++
		}
		sa = servers[normal[a]]
		sb = servers[normal[b]]
	} else if len(danger) == 1 {
		return servers[danger[0]]
	} else if len(danger) == 2 {
		sa = servers[danger[0]]
		sb = servers[danger[1]]
	} else if len(danger) > 2 {
		a := r.Intn(len(danger))
		b := r.Intn(len(danger) - 1)
		if b >= a {
			b++
		}
		sa = servers[danger[a]]
		sb = servers[danger[b]]
	} else if len(nigntmare) == 1 {
		return servers[nigntmare[0]]
	} else if len(nigntmare) == 2 {
		sa = servers[nigntmare[0]]
		sb = servers[nigntmare[1]]
	} else if len(nigntmare) > 2 {
		a := r.Intn(len(nigntmare))
		b := r.Intn(len(nigntmare) - 1)
		if b >= a {
			b++
		}
		sa = servers[nigntmare[a]]
		sb = servers[nigntmare[b]]
	} else {
		return nil
	}
	lsa := math.Sqrt(float64(sa.Pickinfo.Netlag)) * sa.Pickinfo.Cpu * float64(sa.Pickinfo.Activecalls)
	lsb := math.Sqrt(float64(sb.Pickinfo.Netlag)) * sb.Pickinfo.Cpu * float64(sb.Pickinfo.Activecalls)
	if lsa == lsb {
		if sa.Pickinfo.DiscoveryServers < sb.Pickinfo.DiscoveryServers {
			return sb
		}
		return sa
	}
	lsa *= math.Sqrt(float64(uint8(-sa.Pickinfo.DiscoveryServers)))
	lsb *= math.Sqrt(float64(uint8(-sb.Pickinfo.DiscoveryServers)))
	if lsa < lsb {
		return sa
	}
	return sb
}
