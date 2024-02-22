package stream

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/util/common"
)

type connmng struct {
	sendidletimeout time.Duration
	recvidletimeout time.Duration
	heartprobe      time.Duration
	timewheels      []*timewheel
	peernum         int32
	delpeerch       chan *struct{}
	closewait       *sync.WaitGroup
}

type timewheel struct {
	index  uint64
	groups [60]*group
}

type group struct {
	sync.RWMutex
	peers map[string]*Peer
}

func newconnmng(groupnum int, heartprobe, sendidletimeout, recvidletimeout time.Duration) *connmng {
	mng := &connmng{
		sendidletimeout: sendidletimeout,
		recvidletimeout: recvidletimeout,
		heartprobe:      heartprobe,
		timewheels:      make([]*timewheel, groupnum),
		peernum:         0,
		delpeerch:       make(chan *struct{}, 1),
		closewait:       &sync.WaitGroup{},
	}
	for i := 0; i < groupnum; i++ {
		tw := &timewheel{}
		for j := 0; j < 60; j++ {
			tw.groups[j] = &group{
				peers: make(map[string]*Peer),
			}
		}
		mng.timewheels[i] = tw
	}
	mng.closewait.Add(1)
	go func() {
		defer mng.closewait.Done()
		for {
			<-mng.delpeerch
			if mng.Finished() {
				return
			}
		}
	}()
	for _, v := range mng.timewheels {
		tw := v
		go func() {
			htker := time.NewTicker(mng.heartprobe / 60)
			for {
				t := <-htker.C
				if mng.Finished() {
					htker.Stop()
					return
				}
				newindex := atomic.AddUint64(&tw.index, 1)
				//give 1/3 heartprobe for net lag
				g := tw.groups[newindex%60]
				go g.checkheart(mng.heartprobe*3+mng.heartprobe/3, mng.sendidletimeout, mng.recvidletimeout, &t)
			}
		}()
	}
	return mng
}

var errDup = errors.New("duplicate connection")
var errClosing = errors.New("instance closing")

func (m *connmng) AddPeer(p *Peer) error {
	peeraddr := p.c.RemoteAddr().String()
	tw := m.timewheels[common.Bkdrhash(common.STB(peeraddr), uint64(len(m.timewheels)))]
	g := tw.groups[(atomic.LoadUint64(&tw.index)+rand.Uint64()%50+5)%60] //rand is used to reduce race
	g.Lock()
	if _, ok := g.peers[peeraddr]; ok {
		g.Unlock()
		return errDup
	}
	p.peergroup = g
	for {
		old := atomic.LoadInt32(&m.peernum)
		if old < 0 {
			g.Unlock()
			return errClosing
		}
		if atomic.CompareAndSwapInt32(&m.peernum, old, old+1) {
			g.peers[peeraddr] = p
			break
		}
	}
	g.Unlock()
	return nil
}
func (m *connmng) DelPeer(p *Peer) {
	p.peergroup.Lock()
	delete(p.peergroup.peers, p.c.RemoteAddr().String())
	p.peergroup.Unlock()
	p.peergroup = nil
	atomic.AddInt32(&m.peernum, -1)
	select {
	case m.delpeerch <- nil:
	default:
	}
}

// new connections failed
// old connections working
func (m *connmng) PreStop() {
	for {
		old := atomic.LoadInt32(&m.peernum)
		if old < 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&m.peernum, old, old-math.MaxInt32) {
			return
		}
	}
}

// new connections failed
// old connections closed
func (m *connmng) Stop() {
	defer m.closewait.Wait()
	for {
		if old := atomic.LoadInt32(&m.peernum); old >= 0 {
			if atomic.CompareAndSwapInt32(&m.peernum, old, old-math.MaxInt32) {
				break
			}
		} else {
			break
		}
	}
	for _, tw := range m.timewheels {
		for _, g := range tw.groups {
			g.Lock()
			for _, p := range g.peers {
				p.Close()
			}
			g.Unlock()
		}
	}
	//prevent there are no peers
	select {
	case m.delpeerch <- nil:
	default:
	}
}
func (m *connmng) GetPeerNum() int32 {
	peernum := atomic.LoadInt32(&m.peernum)
	if peernum >= 0 {
		return peernum
	} else {
		return peernum + math.MaxInt32
	}
}
func (m *connmng) Finishing() bool {
	return atomic.LoadInt32(&m.peernum) < 0
}
func (m *connmng) Finished() bool {
	return atomic.LoadInt32(&m.peernum) == -math.MaxInt32
}
func (m *connmng) RangePeers(handler func(p *Peer)) {
	wg := sync.WaitGroup{}
	for _, tw := range m.timewheels {
		for _, g := range tw.groups {
			g.RLock()
			wg.Add(len(g.peers))
			for _, p := range g.peers {
				go func(p *Peer) {
					handler(p)
					wg.Done()
				}(p)
			}
			g.RUnlock()
		}
	}
	wg.Wait()
}

func (g *group) checkheart(hearttimeout, sendidletimeout, recvidletimeout time.Duration, now *time.Time) {
	g.RLock()
	for _, p := range g.peers {
		p.checkheart(hearttimeout, sendidletimeout, recvidletimeout, now)
	}
	g.RUnlock()
}
