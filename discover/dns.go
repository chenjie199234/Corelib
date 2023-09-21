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
	crpcport  int
	cgrpcport int
	webport   int
	interval  time.Duration
	notices   map[chan *struct{}]*struct{}
	triger    chan *struct{}
	status    int32 //0-idle,1-discover,2-stop

	sync.RWMutex
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
		crpcport:  crpcport,
		cgrpcport: cgrpcport,
		webport:   webport,
		interval:  interval,
		notices:   make(map[chan *struct{}]*struct{}, 1),
		triger:    make(chan *struct{}, 1),
	}
	d.triger <- nil
	d.status = 1
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

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
func (d *DnsD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.Lock()
	d.notices[ch] = nil
	d.Unlock()
	if atomic.LoadInt32(&d.status) == 0 {
		select {
		case ch <- nil:
		default:
		}
	}
	return ch, func() {
		d.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.Unlock()
	}
}

func (d *DnsD) GetAddrs(pt PortType) (map[string]*RegisterData, Version, error) {
	d.RLock()
	defer d.RUnlock()
	r := make(map[string]*RegisterData)
	reg := &RegisterData{
		DServers: map[string]*struct{}{"dns": nil},
		Addition: nil,
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
func (d *DnsD) Stop() {
	if atomic.SwapInt32(&d.status, 2) == 2 {
		return
	}
	select {
	case d.triger <- nil:
	default:
	}
}
func (d *DnsD) run() {
	defer func() {
		d.Lock()
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
		d.Unlock()
	}()
	tker := time.NewTicker(d.interval)
	for {
		select {
		case <-d.triger:
		case <-tker.C:
		}
		if atomic.LoadInt32(&d.status) == 2 {
			log.Info(nil, "[discover.dns] discover stopped", map[string]interface{}{"host": d.host, "interval": ctime.Duration(d.interval)})
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		atomic.CompareAndSwapInt32(&d.status, 0, 1)
		addrs, e := net.LookupHost(d.host)
		if e != nil {
			log.Error(nil, "[discover.dns] look up failed", map[string]interface{}{"host": d.host, "error": e})
			d.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.Unlock()
			continue
		}
		d.Lock()
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
		d.Unlock()
		for len(tker.C) > 0 {
			<-tker.C
		}
	}
}
func (d *DnsD) CheckTarget(target string) bool {
	return target == d.target
}
