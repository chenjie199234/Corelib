package discover

import (
	"strconv"
	"strings"
	"sync"
)

type DirectD struct {
	app       string
	addr      string
	crpcport  int
	cgrpcport int
	webport   int
	notices   map[chan *struct{}]*struct{}

	sync.RWMutex
}

// addr can be host/ipv4/ipv6
func NewDirectDiscover(targetproject, targetgroup, targetapp, addr string, crpcport, cgrpcport, webport int) DI {
	return &DirectD{
		app:       targetproject + "-" + targetgroup + "." + targetapp,
		addr:      addr,
		crpcport:  crpcport,
		cgrpcport: cgrpcport,
		webport:   webport,
		notices:   make(map[chan *struct{}]*struct{}),
	}
}

func (d *DirectD) Now() {
	d.RLock()
	defer d.RUnlock()
	for notice := range d.notices {
		notice <- nil
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *DirectD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.Lock()
	d.notices[ch] = nil
	d.Unlock()
	return ch, func() {
		d.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.Unlock()
	}
}
func (d *DirectD) GetAddrs(pt PortType) (map[string]*RegisterData, error) {
	d.RLock()
	defer d.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"direct": nil},
		Addition: nil,
	}
	switch pt {
	case NotNeed:
		r[d.addr] = reg
	case Crpc:
		if d.crpcport > 0 {
			if !strings.Contains(d.addr, ":") {
				//treat as host or ipv4
				r[d.addr+":"+strconv.Itoa(d.crpcport)] = reg
			} else {
				//treat as ipv6
				r["["+d.addr+"]:"+strconv.Itoa(d.crpcport)] = reg
			}
		} else {
			r[d.addr] = reg
		}
	case Cgrpc:
		if d.cgrpcport > 0 {
			if !strings.Contains(d.addr, ":") {
				//treat as host or ipv4
				r[d.addr+":"+strconv.Itoa(d.cgrpcport)] = reg
			} else {
				//treat as ipv6
				r["["+d.addr+"]:"+strconv.Itoa(d.cgrpcport)] = reg
			}
		} else {
			r[d.addr] = reg
		}
	case Web:
		if d.webport > 0 {
			if !strings.Contains(d.addr, ":") {
				//treat as host or ipv4
				r[d.addr+":"+strconv.Itoa(d.webport)] = reg
			} else {
				//treat as ipv6
				r["["+d.addr+"]:"+strconv.Itoa(d.webport)] = reg
			}
		} else {
			r[d.addr] = reg
		}
	}
	return r, nil
}
func (d *DirectD) Stop() {
	d.Lock()
	defer d.Unlock()
	for notice := range d.notices {
		delete(d.notices, notice)
		close(notice)
	}
}
func (d *DirectD) CheckApp(app string) bool {
	return app == d.app
}
