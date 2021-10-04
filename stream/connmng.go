package stream

import (
	"context"
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
	stopch          chan struct{}
}

type timewheel struct {
	index uint
	wheel [50]*group
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
		stopch:          make(chan struct{}),
	}
	for i := 0; i < groupnum; i++ {
		tw := &timewheel{}
		for j := 0; j < 50; j++ {
			tw.wheel[j] = &group{
				peers: make(map[string]*Peer),
			}
		}
		mng.groups[i] = tw
	}
	go mng.run()
	return mng
}

var errDupConnection = errors.New("dup connection")
var errServerClosed = errors.New("server closed")

func (m *connmng) AddPeer(p *Peer) error {
	uniquename := p.getUniqueName()
	tw := m.groups[common.BkdrhashString(uniquename, uint64(len(m.groups)))]
	g := tw.wheel[(tw.index+uint(rand.Intn(40))+5)%50] //rand is used to reduce race
	g.Lock()
	if _, ok := g.peers[uniquename]; ok {
		g.Unlock()
		return errDupConnection
	}
	p.peergroup = g
	for {
		old := m.peernum
		if old < 0 {
			g.Unlock()
			return errServerClosed
		}
		if atomic.CompareAndSwapInt32(&m.peernum, old, old+1) {
			g.peers[uniquename] = p
			break
		}
	}
	g.Unlock()
	return nil
}
func (m *connmng) DelPeer(p *Peer) {
	p.peergroup.Lock()
	delete(p.peergroup.peers, p.getUniqueName())
	p.peergroup.Unlock()
	p.peergroup = nil
	atomic.AddInt32(&m.peernum, -1)
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
	return m.peernum < 0
}
func (m *connmng) Finished() bool {
	return m.peernum == -math.MaxInt32
}
func (m *connmng) run() {
	for _, v := range m.groups {
		tw := v
		go func() {
			tker := time.NewTicker(m.heartprobe / 50)
			for {
				select {
				case t := <-tker.C:
					tw.index++
					//give 1/3 heartprobe for net lag
					go tw.wheel[tw.index%50].run(m.heartprobe*3+m.heartprobe/3, m.sendidletimeout, m.recvidletimeout, &t)
				case <-m.stopch:
					return
				}
			}
		}()
	}
}
func (m *connmng) SendMessage(ctx context.Context, data []byte, block bool) {
	for _, tw := range m.groups {
		for _, g := range tw.wheel {
			g.SendMessage(ctx, data, block)
		}
	}
}
func (m *connmng) stop() bool {
	stop := false
	for {
		old := m.peernum
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&m.peernum, old, old-math.MaxInt32) {
				stop = true
				break
			}
		} else {
			break
		}
	}
	if stop {
		close(m.stopch)
		for _, tw := range m.groups {
			for _, g := range tw.wheel {
				g.Lock()
				for _, p := range g.peers {
					p.Close(p.sid)
				}
				g.Unlock()
			}
		}
	}
	return stop
}
func (g *group) run(hearttimeout, sendidletimeout, recvidletimeout time.Duration, now *time.Time) {
	g.Lock()
	defer g.Unlock()
	for _, p := range g.peers {
		p.checkheart(hearttimeout, sendidletimeout, recvidletimeout, now)
	}
}
func (g *group) SendMessage(ctx context.Context, data []byte, block bool) {
	wg := sync.WaitGroup{}
	g.RLock()
	wg.Add(len(g.peers))
	for _, p := range g.peers {
		go func(sender *Peer) {
			sender.SendMessage(ctx, data, sender.sid, block)
			wg.Done()
		}(p)
	}
	g.RUnlock()
	wg.Wait()
}
