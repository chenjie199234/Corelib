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
	groups          []*timewheel
	peernum         int32
	delpeerch       chan *struct{}
	closewait       *sync.WaitGroup
}

type timewheel struct {
	index uint64
	wheel [20]*group
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
		groups:          make([]*timewheel, groupnum),
		peernum:         0,
		delpeerch:       make(chan *struct{}, 1),
		closewait:       &sync.WaitGroup{},
	}
	for i := 0; i < groupnum; i++ {
		tw := &timewheel{}
		for j := 0; j < 20; j++ {
			tw.wheel[j] = &group{
				peers: make(map[string]*Peer),
			}
		}
		mng.groups[i] = tw
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
	for _, v := range mng.groups {
		tw := v
		go func() {
			tker := time.NewTicker(mng.heartprobe / 20)
			for {
				t := <-tker.C
				if mng.Finished() {
					tker.Stop()
					return
				}
				newindex := atomic.AddUint64(&tw.index, 1)
				//give 1/3 heartprobe for net lag
				g := tw.wheel[newindex%20]
				go g.run(mng.heartprobe*3+mng.heartprobe/3, mng.sendidletimeout, mng.recvidletimeout, &t)
			}
		}()
	}
	return mng
}

var errDup = errors.New("duplicate connection")
var errClosing = errors.New("instance closing")

func (m *connmng) AddPeer(p *Peer) error {
	peeraddr := p.c.RemoteAddr().String()
	tw := m.groups[common.BkdrhashString(peeraddr, uint64(len(m.groups)))]
	g := tw.wheel[(atomic.LoadUint64(&tw.index)+rand.Uint64()%16+2)%20] //rand is used to reduce race
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
	for _, tw := range m.groups {
		for _, g := range tw.wheel {
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
	for _, tw := range m.groups {
		for _, g := range tw.wheel {
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
func (g *group) run(hearttimeout, sendidletimeout, recvidletimeout time.Duration, now *time.Time) {
	g.RLock()
	for _, p := range g.peers {
		p.checkheart(hearttimeout, sendidletimeout, recvidletimeout, now)
	}
	g.RUnlock()
}
