package picker

import (
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/container/ring"
)

type ServerForPick interface {
	Pickable() bool
	GetServerPickInfo() *ServerPickInfo //if this return nil,this server will nerver be picked
	GetServerAddr() string              //if this return empty,this server will nerver be picked by forceaddr
}

func NewServerPickInfo() *ServerPickInfo {
	return &ServerPickInfo{
		sf:    ring.NewRing[bool](1000),
		ctime: ring.NewRing[uint64](1000),
	}
}

type ServerPickInfo struct {
	discoverServerNum              uint32
	discoverServerOfflineTimestamp int64
	activecalls                    atomic.Uint32      //current active calls
	sf                             *ring.Ring[bool]   //the last 1000 calls' success and fail status
	sfdata                         uint64             //right 32bit is success count,left 32bit is fail count
	ctime                          *ring.Ring[uint64] //the last 1000 calls' cost time,unixnano
	timedata                       uint64             //right 10bit is count,left 54bit is the total call time for the last 1000 calls
}

func (spi *ServerPickInfo) Pick() {
	//add active call num
	spi.activecalls.Add(1)
}

// costtime is unixnano
func (spi *ServerPickInfo) Done(success bool, costtime uint64) {
	if costtime != 0 {
		for {
			if spi.ctime.Push(costtime) {
				spi.addcosttime(costtime)
				break
			} else if data, e := spi.ctime.Pop(nil); e == ring.ErrPopEmpty {
				continue
			} else {
				spi.delcosttime(data)
			}
		}
	}
	if success {
		for {
			if spi.sf.Push(true) {
				spi.addsuccess()
				break
			} else if data, e := spi.sf.Pop(nil); e == ring.ErrPopEmpty {
				continue
			} else if data {
				spi.delsuccess()
			} else {
				spi.delfail()
			}
		}
	} else {
		for {
			if spi.sf.Push(false) {
				spi.addfail()
				break
			} else if data, e := spi.sf.Pop(nil); e == ring.ErrPopEmpty {
				continue
			} else if data {
				spi.delsuccess()
			} else {
				spi.delfail()
			}
		}
	}
	//reduce active call num
	spi.activecalls.Add(math.MaxUint32)
}

func (spi *ServerPickInfo) GetActiveCalls() uint32 {
	return spi.activecalls.Load()
}

// if DiscoverServerNum > 0
//
//	this server's status will be set to normal
//	the pick priority is depend on the load
//
// if DiscoverServerNum == 0
//
//	this server's status will be set to nightmare(see SetDiscoverServerOffline function)
func (spi *ServerPickInfo) SetDiscoverServerOnline(DiscoverServerNum uint32) {
	if DiscoverServerNum == 0 {
		spi.SetDiscoverServerOffline(DiscoverServerNum)
		return
	}
	atomic.StoreUint32(&spi.discoverServerNum, DiscoverServerNum)
	atomic.StoreInt64(&spi.discoverServerOfflineTimestamp, 0)
}

// if DiscoverServerNum > 0
//
//	this server's status will be set to danger
//	the pick priority will be reduced when status is danger
//	the status will turn back to normal in 1s
//
// if DiscoverServerNum == 0
//
//	this server's status wil be set to nightmare
//	the pick priority will fall to 0
//	the pick priority will not recover unless the SetDiscoverServerOnline is called
func (spi *ServerPickInfo) SetDiscoverServerOffline(DiscoverServerNum uint32) {
	atomic.StoreUint32(&spi.discoverServerNum, DiscoverServerNum)
	atomic.StoreInt64(&spi.discoverServerOfflineTimestamp, time.Now().UnixNano())
}

func (spi *ServerPickInfo) getsuccessfail() (success uint32, fail uint32) {
	successfail := atomic.LoadUint64(&spi.sfdata)
	success = uint32(successfail & uint64(math.MaxUint32))
	fail = uint32(successfail >> 32)
	return
}
func (spi *ServerPickInfo) addsuccess() {
	atomic.AddUint64(&spi.sfdata, 1)
}
func (spi *ServerPickInfo) delsuccess() {
	atomic.AddUint64(&spi.sfdata, math.MaxUint64)
}
func (spi *ServerPickInfo) addfail() {
	atomic.AddUint64(&spi.sfdata, 1<<32)
}
func (spi *ServerPickInfo) delfail() {
	atomic.AddUint64(&spi.sfdata, math.MaxUint64-1<<32+1)
}
func (spi *ServerPickInfo) getcosttime() (totaltime uint64, count uint64) {
	timedata := atomic.LoadUint64(&spi.timedata)
	totaltime = timedata >> 10
	count = (timedata << 54) >> 54
	return
}
func (spi *ServerPickInfo) addcosttime(costtime uint64) {
	atomic.AddUint64(&spi.timedata, costtime<<10+1)
}
func (spi *ServerPickInfo) delcosttime(costtime uint64) {
	atomic.AddUint64(&spi.timedata, math.MaxUint64-(costtime<<10+1)+1)
}
func (spi *ServerPickInfo) getload() float64 {
	success, fail := spi.getsuccessfail()
	costtime, count := spi.getcosttime()
	activecalls := spi.activecalls.Load()
	return math.Atan(math.Sqrt(math.Cbrt(float64(activecalls)))) *
		math.Atan(math.Sqrt(math.Cbrt(float64(costtime)/float64(count)))) *
		(1 + float64(fail)/float64(success+fail))
}

type Picker struct {
	servers []ServerForPick
}

func NewPicker(servers []ServerForPick) *Picker {
	return &Picker{servers: servers}
}

func (p *Picker) ServerLen() int {
	return len(p.servers)
}

// if the forceaddr is not empty,picker will try to return this specific addr's server,if not exist,nil will return
func (p *Picker) Pick(forceaddr string) ServerForPick {
	if len(p.servers) == 0 {
		return nil
	}
	if forceaddr != "" {
		for _, v := range p.servers {
			s := v
			saddr := s.GetServerAddr()
			if saddr != forceaddr {
				continue
			}
			if !s.Pickable() {
				return nil
			}
			info := s.GetServerPickInfo()
			if info == nil {
				return nil
			}
			info.Pick()
			return s
		}
		return nil
	}
	var normal1, normal2, danger1, danger2, nightmare1, nightmare2 ServerForPick
	startindex := rand.Intn(len(p.servers))
	endindex := startindex
	now := time.Now()
	for {
		if p.servers[startindex].Pickable() {
			if info := p.servers[startindex].GetServerPickInfo(); info != nil {
				if info.discoverServerNum <= 0 {
					//nightmare
					if nightmare1 == nil {
						nightmare1 = p.servers[startindex]
					} else if nightmare2 == nil {
						nightmare2 = p.servers[startindex]
					}
				} else if info.discoverServerOfflineTimestamp > 0 && now.UnixNano()-info.discoverServerOfflineTimestamp < time.Second.Nanoseconds() {
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
	var s ServerForPick
	if normal2 != nil {
		//normal 1 and normal 2 both exist
		s = p.compare(normal1, normal2)
	} else if normal1 != nil {
		//only exist normal 1
		s = normal1
	} else if danger2 != nil {
		//danger 1 and danger 2 both exist
		s = p.compare(danger1, danger2)
	} else if danger1 != nil {
		//only exist danger 1
		s = danger1
	} else if nightmare2 != nil {
		//nightmare 1 and nightmare 2 both exist
		s = p.compare(nightmare1, nightmare2)
	} else if nightmare1 != nil {
		//only exist nightmare 1
		s = nightmare1
	}
	if s == nil {
		return nil
	}
	s.GetServerPickInfo().Pick()
	return s
}

func (p *Picker) compare(a, b ServerForPick) ServerForPick {
	aload := a.GetServerPickInfo().getload()
	bload := b.GetServerPickInfo().getload()
	if aload < bload {
		return a
	} else if aload > bload {
		return b
	} else if rand.Intn(2) == 0 {
		return a
	} else {
		return b
	}
}
