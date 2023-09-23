package discover

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/name"
)

type StaticD struct {
	target    string
	crpcport  int
	cgrpcport int
	webport   int
	notices   map[chan *struct{}]*struct{}

	sync.RWMutex
	addrs     []string
	version   int64
	lasterror error
}

// addrs' key can be host(no scheme)/ipv4/ipv6
func NewStaticDiscover(targetproject, targetgroup, targetapp string, addrs []string, crpcport, cgrpcport, webport int) (DI, error) {
	targetfullname, e := name.MakeFullName(targetproject, targetgroup, targetapp)
	if e != nil {
		return nil, e
	}
	return &StaticD{
		target:    targetfullname,
		addrs:     addrs,
		version:   time.Now().UnixNano(),
		crpcport:  crpcport,
		cgrpcport: cgrpcport,
		webport:   webport,
		notices:   make(map[chan *struct{}]*struct{}),
	}, nil
}

func (d *StaticD) Now() {
	d.RLock()
	defer d.RUnlock()
	for notice := range d.notices {
		notice <- nil
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *StaticD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	ch <- nil
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
func (d *StaticD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.RLock()
	defer d.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"static": nil},
		Addition: nil,
	}
	for _, addr := range d.addrs {
		switch pt {
		case NotNeed:
		case Crpc:
			if d.crpcport > 0 {
				if !strings.Contains(addr, ":") {
					//treat as host or ipv4
					addr = addr + ":" + strconv.Itoa(d.crpcport)
				} else {
					//treat as ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.crpcport)
				}
			}
		case Cgrpc:
			if d.cgrpcport > 0 {
				if !strings.Contains(addr, ":") {
					//treat as host or ipv4
					addr = addr + ":" + strconv.Itoa(d.cgrpcport)
				} else {
					//treat as ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.cgrpcport)
				}
			}
		case Web:
			if d.webport > 0 {
				if !strings.Contains(addr, ":") {
					//treat as host or ipv4
					addr = addr + ":" + strconv.Itoa(d.webport)
				} else {
					//treat as ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.webport)
				}
			}
		}
		r[addr] = reg
	}
	return r, d.version, d.lasterror
}
func (d *StaticD) Stop() {
	d.Lock()
	defer d.Unlock()
	if d.lasterror != cerror.ErrDiscoverStopped {
		log.Info(nil, "[discover.static] discover stopped", log.Any("addrs", d.addrs))
		d.lasterror = cerror.ErrDiscoverStopped
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
	}
}
func (d *StaticD) CheckTarget(target string) bool {
	return target == d.target
}
