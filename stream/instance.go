package stream

import (
	"errors"
	"net"
	"sync"
)

const maxPieceLen = 65500 //ensure every piece of data < 64k

type Instance struct {
	c *InstanceConfig

	sync.Mutex
	listeners []*net.TCPListener
	mng       *connmng
}

func NewInstance(c *InstanceConfig) (*Instance, error) {
	if c == nil {
		return nil, errors.New("[Stream.NewInstance] config is nil")
	}
	//verify func can't be nill
	//user data deal func can't be nill
	//online and offline func can be nill
	if c.VerifyFunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing verify function")
	}
	if c.UserdataFunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing userdata function")
	}
	c.validate()
	stream := &Instance{
		c:         c,
		listeners: make([]*net.TCPListener, 0),
		mng:       newconnmng(c.GroupNum, c.HeartprobeInterval, c.SendIdleTimeout, c.RecvIdleTimeout),
	}
	return stream, nil
}

// new connections failed
// old connections working
// WARN: this will cause StartServer return
func (this *Instance) PreStop() {
	this.mng.PreStop()
	this.Lock()
	for _, listener := range this.listeners {
		listener.Close()
	}
	this.Unlock()
}

// new connections failed
// old connections closed
// WARN: this will cause StartServer return
func (this *Instance) Stop() {
	this.mng.Stop()
	this.Lock()
	for _, listener := range this.listeners {
		listener.Close()
	}
	this.Unlock()
}
func (this *Instance) GetPeerNum() int32 {
	return this.mng.GetPeerNum()
}
func (this *Instance) RangePeers(block bool, handler func(p *Peer)) {
	this.mng.RangePeers(block, handler)
}
func (this *Instance) GetPeer(uniqueid string) *Peer {
	return this.mng.GetPeer(uniqueid)
}
func (this *Instance) KickPeer(block bool, uniqueid string) {
	if p := this.mng.GetPeer(uniqueid); p != nil {
		p.Close(block)
	}
}
