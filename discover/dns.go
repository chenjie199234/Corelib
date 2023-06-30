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
)

type DnsD struct {
	app       string
	host      string
	crpcport  int
	cgrpcport int
	webport   int
	interval  time.Duration
	notices   map[chan *struct{}]*struct{}
	triger    chan *struct{}
	status    int32 //0-idle,1-discover,2-stop

	sync.RWMutex
	addrs     map[string]*RegisterData
	lasterror error
}

// interval min is 1s,default is 10s
// if silent is true,means no logs
func NewDNSDiscover(targetappgroup, targetappname, host string, interval time.Duration, crpcport, cgrpcport, webport int) DI {
	if interval < time.Second {
		interval = time.Second * 10
	}
	d := &DnsD{
		app:       targetappgroup + "." + targetappname,
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
	return d
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
	return ch, func() {
		d.Lock()
		if _, ok := d.notices[ch]; ok {
			delete(d.notices, ch)
			close(ch)
		}
		d.Unlock()
	}
}

func (d *DnsD) GetAddrs(pt PortType) (map[string]*RegisterData, error) {
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
				if strings.Contains(k, ":") {
					//ipv6
					k = "[" + k + "]:" + strconv.Itoa(d.crpcport)
				} else {
					//ipv4
					k = k + ":" + strconv.Itoa(d.crpcport)
				}
			}
		case Cgrpc:
			if d.cgrpcport > 0 {
				if strings.Contains(k, ":") {
					//ipv6
					k = "[" + k + "]:" + strconv.Itoa(d.cgrpcport)
				} else {
					//ipv4
					k = k + ":" + strconv.Itoa(d.cgrpcport)
				}
			}
		case Web:
			if d.webport > 0 {
				if strings.Contains(k, ":") {
					//ipv6
					k = "[" + k + "]:" + strconv.Itoa(d.webport)
				} else {
					//ipv4
					k = k + ":" + strconv.Itoa(d.webport)
				}
			}
		}
		r[k] = v
	}
	return r, d.lasterror
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
			log.Info(nil, "[discover.dns] host:", d.host, cerror.ErrDiscoverStopped)
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		atomic.CompareAndSwapInt32(&d.status, 0, 1)
		addrs, e := net.LookupHost(d.host)
		if e != nil {
			log.Error(nil, "[discover.dns] host:", d.host, e)
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
		tmp := make(map[string]*RegisterData)
		for _, addr := range addrs {
			tmp[addr] = &RegisterData{
				DServers: map[string]*struct{}{"dns": nil},
				Addition: nil,
			}
		}
		d.Lock()
		d.addrs = tmp
		d.lasterror = nil
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
		atomic.CompareAndSwapInt32(&d.status, 1, 0)
	}
}
func (d *DnsD) CheckApp(app string) bool {
	return app == d.app
}
