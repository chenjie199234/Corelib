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
	lker      *sync.RWMutex
	notices   map[chan *struct{}]*struct{}
	crpcport  int
	cgrpcport int
	webport   int
	addrs     []string
	version   int64
	lasterror error
}

// addrs' key can be host(no scheme)/ipv4/ipv6
func NewStaticDiscover(targetproject, targetgroup, targetapp string, addrs []string, crpcport, cgrpcport, webport int) (DI, error) {
	undup := make(map[string]*struct{}, len(addrs))
	for _, addr := range addrs {
		undup[addr] = nil
	}
	addrs = make([]string, 0, len(undup))
	for k := range undup {
		addrs = append(addrs, k)
	}
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
		lker:      &sync.RWMutex{},
	}, nil
}

func (d *StaticD) Now() {
	d.lker.RLock()
	defer d.lker.RUnlock()
	for notice := range d.notices {
		select {
		case notice <- nil:
		default:
		}
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *StaticD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.lker.Lock()
	if d.lasterror == cerror.ErrDiscoverStopped {
		close(ch)
	} else {
		ch <- nil
		d.notices[ch] = nil
	}
	d.lker.Unlock()
	return ch, func() {
		d.lker.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.lker.Unlock()
	}
}
func (d *StaticD) UpdateAddrs(addrs []string, crpcport, cgrpcport, webport int) {
	undup := make(map[string]*struct{}, len(addrs))
	for _, addr := range addrs {
		undup[addr] = nil
	}
	addrs = make([]string, 0, len(undup))
	for k := range undup {
		addrs = append(addrs, k)
	}
	d.lker.Lock()
	defer d.lker.Unlock()
	changed := d.crpcport != crpcport ||
		d.cgrpcport != cgrpcport ||
		d.webport != webport ||
		len(d.addrs) != len(addrs)
	if !changed {
		for i := range d.addrs {
			find := false
			for j := range addrs {
				if d.addrs[i] == addrs[j] {
					find = true
					break
				}
			}
			if !find {
				changed = true
				break
			}
		}
	}
	if changed {
		d.addrs = addrs
		d.version = time.Now().UnixNano()
		d.crpcport = crpcport
		d.cgrpcport = cgrpcport
		d.webport = webport
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
	}
}
func (d *StaticD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.lker.RLock()
	defer d.lker.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"static": nil},
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
	d.lker.Lock()
	defer d.lker.Unlock()
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
