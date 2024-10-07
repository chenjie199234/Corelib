package discover

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
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
	status    int32 //0 idle,1 discovering

	ctx    context.Context
	cancel context.CancelFunc

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
	d.ctx, d.cancel = context.WithCancel(context.Background())
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
	d.cancel()
}

// don't close the returned channel,it will be closed in cases:
// 1.the cancel function be called
// 2.this discover stopped
// don't forget to call the returned cancel function if u don't need the notice channel
func (d *DnsD) GetNotice() (notice <-chan *struct{}, cancel func()) {
	ch := make(chan *struct{}, 1)
	d.lker.Lock()
	if d.status == 0 {
		ch <- nil
		d.notices[ch] = nil
	} else {
		select {
		case <-d.ctx.Done():
			close(ch)
		default:
			d.notices[ch] = nil
		}
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
	tker := time.NewTicker(d.interval)
	defer func() {
		tker.Stop()
		d.lker.Lock()
		for notice := range d.notices {
			delete(d.notices, notice)
			close(notice)
		}
		d.lker.Unlock()
	}()
	for {
		select {
		case <-d.ctx.Done():
			slog.InfoContext(nil, "[discover.dns] discover stopped",
				slog.String("target", d.target),
				slog.String("host", d.host),
				slog.Duration("interval", d.interval))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		case <-d.triger:
		case <-tker.C:
		}
		d.status = 1
		addrs, e := net.DefaultResolver.LookupHost(d.ctx, d.host)
		if e != nil && cerror.Equal(errors.Unwrap(e), cerror.ErrCanceled) {
			slog.InfoContext(nil, "[discover.dns] discover stopped", slog.String("target", d.target),
				slog.String("host", d.host),
				slog.Duration("interval", d.interval))
			d.lasterror = cerror.ErrDiscoverStopped
			return
		}
		if e != nil {
			slog.ErrorContext(nil, "[discover.dns] look up failed", slog.String("target", d.target),
				slog.String("host", d.host),
				slog.Duration("interval", d.interval),
				slog.String("error", e.Error()))
			d.lker.Lock()
			d.lasterror = e
			for notice := range d.notices {
				select {
				case notice <- nil:
				default:
				}
			}
			d.status = 0
			d.lker.Unlock()
			continue
		}
		d.lker.Lock()
		slices.Sort(addrs)
		if !slices.Equal(addrs, d.addrs) {
			d.addrs = addrs
			d.version = time.Now().UnixNano()
		}
		d.lasterror = nil
		for notice := range d.notices {
			select {
			case notice <- nil:
			default:
			}
		}
		d.status = 0
		d.lker.Unlock()
	}
}
func (d *DnsD) CheckTarget(target string) bool {
	return target == d.target
}
