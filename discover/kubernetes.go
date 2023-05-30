package discover

import (
	"strconv"
	"sync"
	"sync/atomic"
)

type KubernetesD struct {
	silent    bool
	group     string
	app       string
	crpcport  int
	cgrpcport int
	webport   int
	notices   map[chan *struct{}]*struct{}
	stop      chan *struct{}
	status    int32 //0-idle,1-discover,2-stopped

	sync.RWMutex
	addrs     map[string]*RegisterData
	lasterror error
}

func NewKubernetesDiscover(group, app string, crpcport, cgrpcport, webport int, silent bool) DI {
	d := &KubernetesD{
		silent:    silent,
		group:     group,
		app:       app,
		crpcport:  crpcport,
		cgrpcport: cgrpcport,
		webport:   webport,
		notices:   make(map[chan *struct{}]*struct{}, 1),
		stop:      make(chan *struct{}, 1),
	}
	go d.run()
	return d
}
func (d *KubernetesD) Now() {
	//TODO
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *KubernetesD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.Lock()
	defer d.Unlock()
	d.notices[ch] = nil
	return ch, func() {
		d.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.Unlock()
	}
}
func (d *KubernetesD) GetAddrs(pt PortType) (map[string]*RegisterData, error) {
	d.RLock()
	defer d.RUnlock()
	if pt == NotNeed {
		return d.addrs, d.lasterror
	}
	r := make(map[string]*RegisterData)
	for k, v := range d.addrs {
		switch pt {
		case Crpc:
			if d.crpcport > 0 {
				k = k + ":" + strconv.Itoa(d.crpcport)
			}
		case Cgrpc:
			if d.cgrpcport > 0 {
				k = k + ":" + strconv.Itoa(d.cgrpcport)
			}
		case Web:
			if d.webport > 0 {
				k = k + ":" + strconv.Itoa(d.webport)
			}
		}
		r[k] = v
	}
	return r, d.lasterror
}
func (d *KubernetesD) Stop() {
	if atomic.SwapInt32(&d.status, 2) == 2 {
		return
	}
	close(d.stop)
}
func (d *KubernetesD) run() {
	//TODO
}
