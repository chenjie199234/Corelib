package stream

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

type Instance struct {
	selfname   string
	c          *InstanceConfig
	peergroups []*peergroup
	sync.Mutex
	tcplistener  net.Listener
	totalpeernum int32 //'<0'---(closing),'>=0'---(working)

	noticech  chan *Peer
	closewait *sync.WaitGroup

	pool *sync.Pool
}

func (this *Instance) getPeer(writebuffernum, maxmsglen uint32) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	if p, ok := this.pool.Get().(*Peer); ok {
		p.selfmaxmsglen = maxmsglen
		for len(p.writerbuffer) > 0 {
			if v := <-p.writerbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		for len(p.pingpongbuffer) > 0 {
			if v := <-p.pingpongbuffer; v != nil {
				bufpool.PutBuffer(v)
			}
		}
		p.Context = ctx
		p.CancelFunc = cancel
		return p
	}
	p := &Peer{
		selfmaxmsglen:  maxmsglen,
		writerbuffer:   make(chan *bufpool.Buffer, writebuffernum),
		pingpongbuffer: make(chan *bufpool.Buffer, 2),
		Context:        ctx,
		CancelFunc:     cancel,
	}
	return p
}
func (this *Instance) putPeer(p *Peer) {
	p.CancelFunc()
	p.parentgroup = nil
	p.peername = ""
	p.sid = 0
	p.selfmaxmsglen = 0
	p.peermaxmsglen = 0
	for len(p.writerbuffer) > 0 {
		if v := <-p.writerbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	for len(p.pingpongbuffer) > 0 {
		if v := <-p.pingpongbuffer; v != nil {
			bufpool.PutBuffer(v)
		}
	}
	p.conn = nil
	p.lastactive = 0
	p.recvidlestart = 0
	p.sendidlestart = 0
	p.data = nil
	this.pool.Put(p)
}

var errDupConnection = errors.New("dup connection")
var errServerClosed = errors.New("server closed")

func (this *Instance) addPeer(p *Peer) error {
	uniquename := p.getUniqueName()
	group := this.peergroups[common.BkdrhashString(uniquename, uint64(this.c.GroupNum))]
	group.Lock()
	if _, ok := group.peers[uniquename]; ok {
		group.Unlock()
		return errDupConnection
	}
	p.parentgroup = group
	//peer can be add into the group when this instance was not stopped
	for {
		old := this.totalpeernum
		if old < 0 {
			group.Unlock()
			return errServerClosed
		}
		if atomic.CompareAndSwapInt32(&this.totalpeernum, old, old+1) {
			group.peers[uniquename] = p
			break
		}
	}
	group.Unlock()
	return nil
}
func (this *Instance) delPeer(p *Peer) {
	uniquename := p.getUniqueName()
	p.parentgroup.Lock()
	if _, ok := p.parentgroup.peers[uniquename]; ok {
		delete(p.parentgroup.peers, uniquename)
		atomic.AddInt32(&this.totalpeernum, -1)
	}
	p.parentgroup.Unlock()
}

//be careful about the callback func race
func NewInstance(c *InstanceConfig, group, name string) (*Instance, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group+"."+name, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		return nil, errors.New("[Stream.NewInstance] config is nil")
	}
	//verify func can't be nill
	//user data deal func can't be nill
	//online and offline func can be nill
	if c.Verifyfunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing verify function")
	}
	if c.Userdatafunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing userdata function")
	}
	c.validate()
	stream := &Instance{
		selfname:   group + "." + name,
		c:          c,
		peergroups: make([]*peergroup, c.GroupNum),
		noticech:   make(chan *Peer, 1024),
		closewait:  &sync.WaitGroup{},
		pool:       &sync.Pool{},
	}
	stream.closewait.Add(1)
	for i := range stream.peergroups {
		stream.peergroups[i] = &peergroup{
			peers: make(map[string]*Peer, 10),
		}
		go stream.heart(stream.peergroups[i], i)
	}
	go func() {
		for {
			p := <-stream.noticech
			if p != nil {
				stream.delPeer(p)
				stream.putPeer(p)
			}
			if stream.totalpeernum == -math.MaxInt32 {
				stream.closewait.Done()
				return
			}
		}
	}()
	return stream, nil
}
func (this *Instance) Stop() {
	stop := false
	for {
		old := this.totalpeernum
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&this.totalpeernum, old, old-math.MaxInt32) {
				stop = true
				break
			}
		} else {
			break
		}
	}
	if stop {
		if this.tcplistener != nil {
			this.tcplistener.Close()
		}
		for _, group := range this.peergroups {
			group.RLock()
			for _, peer := range group.peers {
				peer.Close(peer.sid)
			}
			group.RUnlock()
		}
		//prevent notice block on empty chan
		select {
		case this.noticech <- nil:
		default:
		}
	}
	this.closewait.Wait()
}
func (this *Instance) GetSelfName() string {
	return this.selfname
}
func (this *Instance) GetPeerNum() int32 {
	totalpeernum := atomic.LoadInt32(&this.totalpeernum)
	if totalpeernum >= 0 {
		return totalpeernum
	} else {
		return totalpeernum + math.MaxInt32
	}
}
func (this *Instance) SendMessageAll(data []byte, block bool) {
	wg := &sync.WaitGroup{}
	for _, group := range this.peergroups {
		group.RWMutex.RLock()
		for _, peer := range group.peers {
			wg.Add(1)
			go func(p *Peer) {
				p.SendMessage(data, p.sid, block)
				wg.Done()
			}(peer)
		}
		group.RWMutex.RUnlock()
	}
	wg.Wait()
}

func (this *Instance) heart(group *peergroup, index int) {
	var tker *time.Ticker
	delay := int64(float64(index) / float64(this.c.GroupNum) * float64(this.c.HeartprobeInterval.Nanoseconds()))
	if delay != 0 {
		time.Sleep(time.Duration(delay))
		tker = time.NewTicker(this.c.HeartprobeInterval)
	} else {
		tker = time.NewTicker(this.c.HeartprobeInterval)
	}
	for {
		<-tker.C
		if this.totalpeernum == -math.MaxInt32 {
			return
		}
		now := time.Now().UnixNano()
		group.RLock()
		for _, p := range group.peers {
			if p.sid <= 0 {
				continue
			}
			templastactive := atomic.LoadInt64(&p.lastactive)
			temprecvidlestart := atomic.LoadInt64(&p.recvidlestart)
			tempsendidlestart := atomic.LoadInt64(&p.sendidlestart)
			if now >= templastactive && now-templastactive > int64(this.c.HeartbeatTimeout) {
				//heartbeat timeout
				log.Error(nil, "[Stream.heart] heartbeat timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			if now >= tempsendidlestart && now-tempsendidlestart > int64(this.c.SendIdleTimeout) {
				//send idle timeout
				log.Error(nil, "[Stream.heart] send idle timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			if this.c.RecvIdleTimeout != 0 && now >= temprecvidlestart && now-temprecvidlestart > int64(this.c.RecvIdleTimeout) {
				//recv idle timeout
				log.Error(nil, "[Stream.heart] recv idle timeout:", p.getUniqueName())
				p.closeconn()
				continue
			}
			//send heart beat data
			data := makePingMsg(nil)
			select {
			case p.pingpongbuffer <- data:
			default:
				bufpool.PutBuffer(data)
			}
		}
		group.RUnlock()
	}
}
