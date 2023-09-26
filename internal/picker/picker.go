package picker

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type PI interface {
	ServerLen() int
	UpdateServers([]ServerForPick)
	//if the forceaddr is not empty,picker will try to return this specific addr's server,if not exist,nil will return
	Pick(forceaddr string) (server ServerForPick, done func())
}
type ServerForPick interface {
	//shouldn't return nil
	GetServerPickInfo() *ServerPickInfo
	GetServerAddr() string
}

type ServerPickInfo struct {
	Activecalls uint32 //current active calls
	DServerNum  int32  //this server registered on how many register servers now
	//default should be 0
	//when this server unregister on one register server,this should be set to the timestamp,unit nanosecond(this server maybe is offline,when pick should be careful)
	//when this server register on new register server,this should be set to 0(this server is still online)
	//when this server unregister on old and register on new at the same time,this should be set to 0(this server is changing the register server,means still online)
	DServerOffline int64
	Addition       []byte //addition info register on register servers
}

type Picker struct {
	servers []ServerForPick
}

func NewPicker() PI {
	return &Picker{}
}

func (p *Picker) ServerLen() int {
	return len(p.servers)
}
func (p *Picker) UpdateServers(servers []ServerForPick) {
	p.servers = servers
}

// if the forceaddr is not empty,picker will try to return this specific addr's server,if not exist,nil will return
func (p *Picker) Pick(forceaddr string) (server ServerForPick, done func()) {
	if len(p.servers) == 0 {
		return nil, nil
	}
	if forceaddr != "" {
		for _, v := range p.servers {
			server := v
			serveraddr := server.GetServerAddr()
			var tail string
			if strings.HasPrefix(serveraddr, forceaddr) {
				//ipv4
				tail = serveraddr[len(forceaddr):]
			} else if strings.HasPrefix(serveraddr, "["+forceaddr+"]") {
				//ipv6
				tail = serveraddr[len(forceaddr)+2:]
			} else {
				continue
			}
			//port
			if len(tail) > 0 {
				if tail[0] != ':' {
					continue
				}
				tail = tail[1:]
				if _, e := strconv.Atoi(tail); e != nil {
					continue
				}
			}
			return server, func() { atomic.AddUint32(&(server.GetServerPickInfo().Activecalls), math.MaxUint32) }
		}
		return nil, nil
	}
	var normal1, normal2, danger1, danger2, nightmare1, nightmare2 ServerForPick
	startindex := rand.Intn(len(p.servers))
	endindex := startindex
	now := time.Now()
	for {
		info := p.servers[startindex].GetServerPickInfo()
		if info == nil {
			continue
		}
		if info.DServerNum <= 0 {
			//nightmare
			if nightmare1 == nil {
				nightmare1 = p.servers[startindex]
			} else if nightmare2 == nil {
				nightmare2 = p.servers[startindex]
			}
		} else if info.DServerOffline > 0 && now.UnixNano()-info.DServerOffline < time.Second.Nanoseconds() {
			//danger
			if danger1 == nil {
				danger1 = p.servers[startindex]
			} else if danger2 == nil {
				danger2 = p.servers[startindex]
			}
		} else {
			//normal
			if normal1 == nil {
				normal1 = p.servers[startindex]
			} else {
				normal2 = p.servers[startindex]
				break
			}
		}
		startindex++
		if startindex == len(p.servers) {
			startindex = 0
		}
		if startindex == endindex {
			break
		}
	}
	//1's priority is bigger then the 2
	if normal2 != nil {
		server = p.compare(normal1, normal2, &now)
	} else if normal2 == nil && normal1 != nil {
		server = normal1
	} else if danger2 != nil {
		server = p.compare(danger1, danger2, &now)
	} else if danger2 == nil && danger1 != nil {
		server = danger1
	} else if nightmare2 != nil {
		server = p.compare(nightmare1, nightmare2, &now)
	} else if nightmare2 == nil && nightmare1 != nil {
		server = nightmare1
	}
	atomic.AddUint32(&(server.GetServerPickInfo().Activecalls), 1)
	done = func() { atomic.AddUint32(&(server.GetServerPickInfo().Activecalls), math.MaxUint32) }
	return
}
func (p *Picker) compare(a, b ServerForPick, now *time.Time) ServerForPick {
	ainfo := a.GetServerPickInfo()
	binfo := b.GetServerPickInfo()
	if ainfo.Activecalls < binfo.Activecalls {
		return a
	} else if binfo.Activecalls < ainfo.Activecalls {
		return b
	} else if rand.Intn(2) == 1 {
		return a
	} else {
		return b
	}
}
