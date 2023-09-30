package discover

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/name"
)

type DnsD struct {
	target    string
	host      string
	interval  time.Duration
	crpcport  int
	cgrpcport int
	webport   int
	triger    chan *struct{}
	status    int32

	lker      *sync.RWMutex
	notices   map[chan *struct{}]*struct{}
	addrs     []string
	version   int64
	lasterror error
}

// interval min is 1s,default is 10s
func NewDNSDiscover(targetproject, targetgroup, targetapp, host string, interval time.Duration, crpcport, cgrpcport, webport int) (DI, error) {
	targetfullname, e := name.MakeFullName(targetproject, targetgroup, targetapp)
	if e != nil {
		return nil, e
	}
	if interval < time.Second {
		interval = time.Second * 10
	}
	d := &DnsD{
		target:    targetfullname,
		host:      host,
		interval:  interval,
		crpcport:  crpcport,
		cgrpcport: cgrpcport,
		webport:   webport,
		triger:    make(chan *struct{}, 1),
		status:    1,
		lker:      &sync.RWMutex{},
		notices:   make(map[chan *struct{}]*struct{}, 1),
	}
	d.triger <- nil
	go d.run()
	return d, nil
}

func (d *DnsD) Now() {
	if !atomic.CompareAndSwapInt32(&d.status, 0, 1) {
		return
	}
	select {
	case d.triger <- nil:
	default:
	}
}
func (d *DnsD) Stop() {
	if atomic.SwapInt32(&d.status, 2) == 2 {
		return
	}
	select {
	case d.triger <- nil:
	default:
	}
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *DnsD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.lker.Lock()
	if status := atomic.LoadInt32(&d.status); status == 0 {
		ch <- nil
		d.notices[ch] = nil
	} else if status == 1 {
		d.notices[ch] = nil
	} else {
		close(ch)
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

func (d *DnsD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.lker.RLock()
	defer d.lker.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"dns": nil},
	}
	for _, addr := range d.addrs {
		switch pt {
		case NotNeed:
		case Crpc:
			if d.crpcport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.crpcport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.crpcport)
				}
			}
		case Cgrpc:
			if d.cgrpcport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.cgrpcport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.cgrpcport)
				}
			}
		case Web:
			if d.webport > 0 {
				if strings.Contains(addr, ":") {
					//ipv6
					addr = "[" + addr + "]:" + strconv.Itoa(d.webport)
				} else {
					//ipv4
					addr = addr + ":" + strconv.Itoa(d.webport)
				}
			}
		}
		r[addr] = reg
	}
	return r, d.version, d.lasterror
}

func (d *DnsD) run() {
	defer func() {
		d.lker.Lock()
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
		d.lker.Unlock()
	}()
	tker := time.NewTicker(d.interval)
	for {
		select {
		case <-d.triger:
		case <-tker.C:
		}
		if atomic.LoadInt32(&d.status) == 2 {
			log.Info(nil, "[discover.dns] discover stopped", log.String("host", d.host), log.CDuration("interval", ctime.Duration(d.interval)))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		atomic.CompareAndSwapInt32(&d.status, 0, 1)
		addrs, e := net.LookupHost(d.host)
		if e != nil {
			log.Error(nil, "[discover.dns] look up failed", log.String("host", d.host), log.CDuration("interval", ctime.Duration(d.interval)), log.CError(e))
			d.lker.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.lker.Unlock()
			continue
		}
		d.lker.Lock()
		different := len(addrs) != len(d.addrs)
		if !different {
			for _, newaddr := range addrs {
				find := false
				for _, oldaddr := range d.addrs {
					if newaddr == oldaddr {
						find = true
						break
					}
				}
				if !find {
					different = true
					break
				}
			}
		}
		if different {
			d.addrs = addrs
			d.version = time.Now().UnixNano()
		}
		d.lasterror = nil
		atomic.CompareAndSwapInt32(&d.status, 1, 0)
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
		d.lker.Unlock()
		for len(tker.C) > 0 {
			<-tker.C
		}
	}
}
func (d *DnsD) CheckTarget(target string) bool {
	return target == d.target
}
