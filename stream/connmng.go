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
	groupHeartIndex uint64
	groups          []*group
	peernum         int32
	delpeerch       chan *struct{}
	closewait       *sync.WaitGroup
}

type group struct {
	sync.RWMutex
	peers map[string]*Peer
}

func (g *group) checkheart(hearttimeout, sendidletimeout, recvidletimeout time.Duration, now *time.Time) {
	g.RLock()
	for _, p := range g.peers {
		p.checkheart(hearttimeout, sendidletimeout, recvidletimeout, now)
	}
	g.RUnlock()
}

func newconnmng(groupnum uint16, heartprobe, sendidletimeout, recvidletimeout time.Duration) *connmng {
	mng := &connmng{
		sendidletimeout: sendidletimeout,
		recvidletimeout: recvidletimeout,
		heartprobe:      heartprobe,
		groupHeartIndex: rand.Uint64(),
		groups:          make([]*group, groupnum),
		peernum:         0,
		delpeerch:       make(chan *struct{}, 1),
		closewait:       &sync.WaitGroup{},
	}
	for i := uint16(0); i < groupnum; i++ {
		mng.groups[i] = &group{peers: make(map[string]*Peer)}
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
	go func() {
		timepiece := int64(mng.heartprobe) / int64(groupnum)
		tker := time.NewTicker(time.Duration(timepiece))
		for {
			t := <-tker.C
			if mng.Finished() {
				tker.Stop()
				return
			}
			newindex := atomic.AddUint64(&mng.groupHeartIndex, 1)
			g := mng.groups[newindex%uint64(groupnum)]
			//give 1/3 heartprobe for net lag
			go g.checkheart(mng.heartprobe*3+mng.heartprobe/3, mng.sendidletimeout, mng.recvidletimeout, &t)
		}
	}()
	return mng
}

var errDup = errors.New("duplicate connection")
var errClosing = errors.New("instance closing")

func (m *connmng) AddPeer(p *Peer) error {
	uniqueid := p.GetUniqueID()
	g := m.groups[common.Bkdrhash(common.STB(uniqueid), uint64(len(m.groups)))]
	g.Lock()
	if _, ok := g.peers[uniqueid]; ok {
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
			g.peers[uniqueid] = p
			break
		}
	}
	g.Unlock()
	return nil
}
func (m *connmng) DelPeer(p *Peer) {
	uniqueid := p.GetUniqueID()
	p.peergroup.Lock()
	delete(p.peergroup.peers, uniqueid)
	p.peergroup.Unlock()
	p.peergroup = nil
	close(p.blocknotice)
	atomic.AddInt32(&m.peernum, -1)
	select {
	case m.delpeerch <- nil:
	default:
	}
}
func (m *connmng) GetPeer(uniqueid string) *Peer {
	if uniqueid == "" {
		return nil
	}
	g := m.groups[common.Bkdrhash(common.STB(uniqueid), uint64(len(m.groups)))]
	g.RLock()
	defer g.RUnlock()
	return g.peers[uniqueid]
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
	for _, g := range m.groups {
		g.Lock()
		for _, p := range g.peers {
			p.Close(false)
		}
		g.Unlock()
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
func (m *connmng) RangePeers(block bool, handler func(p *Peer)) {
	if block {
		wg := sync.WaitGroup{}
		for _, g := range m.groups {
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
		wg.Wait()
	} else {
		for _, g := range m.groups {
			g.RLock()
			for _, p := range g.peers {
				go handler(p)
			}
			g.RUnlock()
		}
	}
}
