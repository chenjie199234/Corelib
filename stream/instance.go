package stream

import (
	"errors"
	"net"
	"sync"

	"github.com/chenjie199234/Corelib/util/name"
)

type Instance struct {
	selfappname string //group.name
	c           *InstanceConfig

	sync.Mutex
	listeners []*net.TCPListener
	mng       *connmng
}

func NewInstance(c *InstanceConfig, selfgroup, selfname string) (*Instance, error) {
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
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
		selfappname: selfappname,
		c:           c,
		listeners:   make([]*net.TCPListener, 0),
		mng:         newconnmng(int(c.GroupNum), c.HeartprobeInterval, c.SendIdleTimeout, c.RecvIdleTimeout),
	}
	return stream, nil
}

//new connections failed
//old connections working
//WARN: this will cause StartxxxServer return
func (this *Instance) PreStop() {
	this.mng.PreStop()
	this.Lock()
	for _, listener := range this.listeners {
		listener.Close()
	}
	this.Unlock()
}

//new connections failed
//old connections closed
//WARN: this will cause StartxxxServer return
func (this *Instance) Stop() {
	this.mng.Stop()
	this.Lock()
	for _, listener := range this.listeners {
		listener.Close()
	}
	this.Unlock()
}
func (this *Instance) GetSelfAppName() string {
	return this.selfappname
}
func (this *Instance) GetPeerNum() int32 {
	return this.mng.GetPeerNum()
}
func (this *Instance) RangePeers(handler func(p *Peer)) {
	this.mng.RangePeers(handler)
}
