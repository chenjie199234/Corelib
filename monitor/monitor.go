package monitor

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
)

type monitor struct {
	Sysinfos  []*sysinfo //sorted by sysinfo.Timestamp
	GCInfos   []*gcinfo  //sorted by gcinfo.Timestamp
	lker      *sync.RWMutex
	CallInfos []*callinfo //sorted by callinfo.StartTimestamp
}
type sysinfo struct {
	Timestamp  int64 //nano second
	Routinenum int
	Threadnum  int
	HeapObjnum int
}
type gcinfo struct {
	Timestamp uint64 //nano second
	Timewaste uint64 //nano second
}
type callinfo struct {
	StartTimestamp int64 //nano second
	EndTimestamp   int64 //nano second
	wclker         *sync.Mutex
	Webclient      map[string]map[string]*pathinfo //first key peername,second key path
	wslker         *sync.Mutex
	Webserver      map[string]map[string]*pathinfo //first key peername,second key path
	gclker         *sync.Mutex
	Cgrpcclient    map[string]map[string]*pathinfo //first key peername,second key path
	gslker         *sync.Mutex
	Cgrpcserver    map[string]map[string]*pathinfo //first key peername,second key path
	cclker         *sync.Mutex
	Crpcclient     map[string]map[string]*pathinfo //first key peername,second key path
	cslker         *sync.Mutex
	Crpcserver     map[string]map[string]*pathinfo //first key peername,second key path
}
type pathinfo struct {
	SuccessCount int64
	ErrCount     int64
	ErrCodeCount map[int32]int64
	lker         *sync.Mutex
}

var switcher int64
var lastswitch int64
var m1 *monitor //which >= 0
var m2 *monitor //which < 0

func init() {
	lastswitch = time.Now().UnixNano()
	m1 = &monitor{
		Sysinfos:  make([]*sysinfo, 0, 3),
		GCInfos:   make([]*gcinfo, 0, 10),
		lker:      &sync.RWMutex{},
		CallInfos: make([]*callinfo, 0, 3),
	}
	m1.CallInfos = append(m1.CallInfos, &callinfo{
		StartTimestamp: time.Now().UnixNano(),
		wclker:         &sync.Mutex{},
		Webclient:      make(map[string]map[string]*pathinfo),
		wslker:         &sync.Mutex{},
		Webserver:      make(map[string]map[string]*pathinfo),
		gclker:         &sync.Mutex{},
		Cgrpcclient:    make(map[string]map[string]*pathinfo),
		gslker:         &sync.Mutex{},
		Cgrpcserver:    make(map[string]map[string]*pathinfo),
		cclker:         &sync.Mutex{},
		Crpcclient:     make(map[string]map[string]*pathinfo),
		cslker:         &sync.Mutex{},
		Crpcserver:     make(map[string]map[string]*pathinfo),
	})
	m2 = &monitor{
		Sysinfos:  make([]*sysinfo, 0, 3),
		GCInfos:   make([]*gcinfo, 0, 10),
		lker:      &sync.RWMutex{},
		CallInfos: make([]*callinfo, 0, 3),
	}
	m2.CallInfos = append(m2.CallInfos, &callinfo{
		wclker:      &sync.Mutex{},
		Webclient:   make(map[string]map[string]*pathinfo),
		wslker:      &sync.Mutex{},
		Webserver:   make(map[string]map[string]*pathinfo),
		gclker:      &sync.Mutex{},
		Cgrpcclient: make(map[string]map[string]*pathinfo),
		gslker:      &sync.Mutex{},
		Cgrpcserver: make(map[string]map[string]*pathinfo),
		cclker:      &sync.Mutex{},
		Crpcclient:  make(map[string]map[string]*pathinfo),
		cslker:      &sync.Mutex{},
		Crpcserver:  make(map[string]map[string]*pathinfo),
	})
	go func() {
		tker := time.NewTicker(time.Second * 30)
		for {
			<-tker.C
			sysMonitor()
		}
	}()
}

var lastgcindex uint32

func sysMonitor() {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	now := time.Now().UnixNano()
	//sysinfo
	s := &sysinfo{}
	s.Timestamp = now
	s.Routinenum = runtime.NumGoroutine()
	s.Threadnum, _ = runtime.ThreadCreateProfile(nil)
	meminfo := &runtime.MemStats{}
	runtime.ReadMemStats(meminfo)
	s.HeapObjnum = int(meminfo.HeapObjects)
	m.Sysinfos = append(m.Sysinfos, s)

	//gcinfo
	if meminfo.NumGC > lastgcindex+256 {
		lastgcindex = meminfo.NumGC - 256
	}
	for lastgcindex < meminfo.NumGC {
		tmp := &gcinfo{}
		tmp.Timestamp = meminfo.PauseEnd[lastgcindex%256]
		tmp.Timewaste = meminfo.PauseNs[lastgcindex%256]
		m.GCInfos = append(m.GCInfos, tmp)
		lastgcindex++
	}

	//callinfo
	m.lker.Lock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	if len(cinfo.Webclient)+len(cinfo.Webserver)+len(cinfo.Cgrpcclient)+len(cinfo.Cgrpcserver)+len(cinfo.Crpcclient)+len(cinfo.Crpcserver) == 0 {
		//there's no monitor data,reuse the prev object
		cinfo.StartTimestamp = now
	} else {
		cinfo.EndTimestamp = now
		m.CallInfos = append(m.CallInfos, &callinfo{
			StartTimestamp: now,
			wclker:         &sync.Mutex{},
			Webclient:      make(map[string]map[string]*pathinfo),
			wslker:         &sync.Mutex{},
			Webserver:      make(map[string]map[string]*pathinfo),
			gclker:         &sync.Mutex{},
			Cgrpcclient:    make(map[string]map[string]*pathinfo),
			gslker:         &sync.Mutex{},
			Cgrpcserver:    make(map[string]map[string]*pathinfo),
			cclker:         &sync.Mutex{},
			Crpcclient:     make(map[string]map[string]*pathinfo),
			cslker:         &sync.Mutex{},
			Crpcserver:     make(map[string]map[string]*pathinfo),
		})
	}
	m.lker.Unlock()
}
func WebClientMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.wclker.Lock()
	peer, ok := cinfo.Webclient[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Webclient[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.wclker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func WebServerMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.wslker.Lock()
	peer, ok := cinfo.Webserver[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Webserver[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.wslker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func CgrpcClientMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.gclker.Lock()
	peer, ok := cinfo.Cgrpcclient[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Cgrpcclient[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.gclker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()

}
func CgrpcServerMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.gslker.Lock()
	peer, ok := cinfo.Cgrpcserver[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Cgrpcserver[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.gslker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func CrpcClientMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.cclker.Lock()
	peer, ok := cinfo.Crpcclient[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Crpcclient[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.cclker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func CrpcServerMonitor(peername, path string, e error) {
	var m *monitor
	if atomic.AddInt64(&switcher, 1) > 0 {
		m = m1
	} else {
		m = m2
	}
	defer func() {
		atomic.AddInt64(&switcher, -1)
		//TODO
	}()
	m.lker.RLock()
	cinfo := m.CallInfos[len(m.CallInfos)-1]
	m.lker.RUnlock()
	cinfo.cslker.Lock()
	peer, ok := cinfo.Crpcserver[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Crpcserver[peername] = peer
	}
	pinfo, ok := peer[path]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]int64),
			lker:         &sync.Mutex{},
		}
		peer[path] = pinfo
	}
	cinfo.cslker.Unlock()
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddInt64(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddInt64(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}

var lker sync.Mutex

func getinfo() {
	lker.Lock()
	defer lker.Unlock()

}
