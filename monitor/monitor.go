package monitor

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
)

var m Monitor
var lastclean int64 //prevent too many objects in monitor

var lker sync.RWMutex
var wclker sync.Mutex
var wslker sync.Mutex
var gclker sync.Mutex
var gslker sync.Mutex
var cclker sync.Mutex
var cslker sync.Mutex

type Monitor struct {
	Sysinfos        []*sysinfo  //sorted by sysinfo.Timestamp
	GCinfos         []*gcinfo   //sorted by gcinfo.Timestamp
	WebClientinfos  []*callinfo //sorted by callinfo.StartTimestamp
	WebServerinfos  []*callinfo //sorted by callinfo.StartTimestamp
	GrpcClientinfos []*callinfo //sorted by callinfo.StartTimestamp
	GrpcServerinfos []*callinfo //sorted by callinfo.StartTimestamp
	CrpcClientinfos []*callinfo //sorted by callinfo.StartTimestamp
	CrpcServerinfos []*callinfo //sorted by callinfo.StartTimestamp
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
	StartTimestamp int64                           //nano second
	EndTimestamp   int64                           //nano second
	Callinfos      map[string]map[string]*pathinfo //first key peername,second key path
}
type pathinfo struct {
	SuccessCount uint32
	ErrCount     uint32
	ErrCodeCount map[int32]uint32
	T50          uint64      //nano second
	T90          uint64      //nano second
	T99          uint64      //nano second
	maxTimewaste uint64      //nano second
	timewaste    [114]uint32 //value:count,index:0-9(0ms-10ms) each 1ms,10-27(10ms-100ms) each 5ms,index:28-72(100ms-1s) each 20ms,index:73-112(1s-5s) each 100ms,index:113 more then 5s
	lker         *sync.Mutex
}

func init() {
	refresh(nil)
	go func() {
		tker := time.NewTicker(time.Second * 5)
		for {
			<-tker.C
			sysMonitor()
		}
	}()
}
func refresh(now *time.Time) {
	if now == nil {
		tmp := time.Now()
		now = &tmp
	}
	m.Sysinfos = make([]*sysinfo, 0, 3)
	m.GCinfos = make([]*gcinfo, 0, 10)
	m.WebClientinfos = make([]*callinfo, 0, 24)
	m.WebClientinfos = append(m.WebClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	m.WebServerinfos = make([]*callinfo, 0, 24)
	m.WebServerinfos = append(m.WebServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	m.GrpcClientinfos = make([]*callinfo, 0, 24)
	m.GrpcClientinfos = append(m.GrpcClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	m.GrpcServerinfos = make([]*callinfo, 0, 24)
	m.GrpcServerinfos = append(m.GrpcServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	m.CrpcClientinfos = make([]*callinfo, 0, 24)
	m.CrpcClientinfos = append(m.CrpcClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	m.CrpcServerinfos = make([]*callinfo, 0, 24)
	m.CrpcServerinfos = append(m.CrpcServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	lastclean = now.UnixNano()
}

var lastgcindex uint32

func sysMonitor() {
	lker.Lock()
	defer lker.Unlock()
	now := time.Now()
	if time.Duration(now.UnixNano()-lastclean) > time.Minute*2 {
		//more then 2min,clean the old data
		refresh(&now)
	}
	//sysinfo
	s := &sysinfo{}
	s.Timestamp = now.UnixNano()
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
		m.GCinfos = append(m.GCinfos, tmp)
		lastgcindex++
	}

	//callinfo
	wcinfo := m.WebClientinfos[len(m.WebClientinfos)-1]
	wcinfo.EndTimestamp = now.UnixNano()
	m.WebClientinfos = append(m.WebClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	wsinfo := m.WebServerinfos[len(m.WebServerinfos)-1]
	wsinfo.EndTimestamp = now.UnixNano()
	m.WebServerinfos = append(m.WebServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	gcinfo := m.GrpcClientinfos[len(m.GrpcClientinfos)-1]
	gcinfo.EndTimestamp = now.UnixNano()
	m.GrpcClientinfos = append(m.GrpcClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	gsinfo := m.GrpcServerinfos[len(m.GrpcServerinfos)-1]
	gsinfo.EndTimestamp = now.UnixNano()
	m.GrpcServerinfos = append(m.GrpcServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	ccinfo := m.CrpcClientinfos[len(m.CrpcClientinfos)-1]
	ccinfo.EndTimestamp = now.UnixNano()
	m.CrpcClientinfos = append(m.CrpcClientinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
	csinfo := m.CrpcServerinfos[len(m.CrpcServerinfos)-1]
	csinfo.EndTimestamp = now.UnixNano()
	m.CrpcServerinfos = append(m.CrpcServerinfos, &callinfo{
		StartTimestamp: now.UnixNano(),
		Callinfos:      make(map[string]map[string]*pathinfo),
	})
}
func timewasteIndex(timewaste uint64) int {
	switch {
	case timewaste < uint64(time.Millisecond*10):
		return int((timewaste) / uint64(time.Millisecond))
	case timewaste < uint64(time.Millisecond)*100:
		return 10 + int((timewaste-uint64(time.Millisecond)*10)/(uint64(time.Millisecond)*5))
	case timewaste < uint64(time.Second):
		return 28 + int((timewaste-uint64(time.Millisecond)*100)/(uint64(time.Millisecond)*20))
	case timewaste < uint64(time.Second)*5:
		return 73 + int((timewaste-uint64(time.Second))/(uint64(time.Millisecond)*100))
	default:
		return 113
	}
}
func WebClientMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	wclker.Lock()
	defer wclker.Unlock()
	cinfo := m.WebClientinfos[len(m.WebClientinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func WebServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	wslker.Lock()
	defer wslker.Unlock()
	cinfo := m.WebServerinfos[len(m.WebServerinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func GrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	gclker.Lock()
	defer gclker.Unlock()
	cinfo := m.GrpcClientinfos[len(m.GrpcClientinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()

}
func GrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	gslker.Lock()
	defer gslker.Unlock()
	cinfo := m.GrpcServerinfos[len(m.GrpcServerinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func CrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	cclker.Lock()
	defer cclker.Unlock()
	cinfo := m.CrpcClientinfos[len(m.CrpcClientinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}
func CrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	lker.RLock()
	defer lker.RUnlock()
	cslker.Lock()
	defer cslker.Unlock()
	cinfo := m.CrpcServerinfos[len(m.CrpcServerinfos)-1]
	peer, ok := cinfo.Callinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		cinfo.Callinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	//timewaste
	for {
		oldmax := pinfo.maxTimewaste
		if oldmax >= timewaste {
			break
		}
		if atomic.CompareAndSwapUint64(&pinfo.maxTimewaste, oldmax, timewaste) {
			break
		}
	}
	atomic.AddUint32(&(pinfo.timewaste[timewasteIndex(timewaste)]), 1)
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		atomic.AddUint32(&pinfo.SuccessCount, 1)
		return
	}
	atomic.AddUint32(&pinfo.ErrCount, 1)
	pinfo.lker.Lock()
	pinfo.ErrCodeCount[ee.Code]++
	pinfo.lker.Unlock()
}

func GetMonitorInfo() *Monitor {
	now := time.Now()
	lker.Lock()
	r := &Monitor{
		Sysinfos:        m.Sysinfos,
		GCinfos:         m.GCinfos,
		WebClientinfos:  m.WebClientinfos,
		WebServerinfos:  m.WebServerinfos,
		GrpcClientinfos: m.GrpcClientinfos,
		GrpcServerinfos: m.GrpcServerinfos,
		CrpcClientinfos: m.CrpcClientinfos,
		CrpcServerinfos: m.CrpcServerinfos,
	}
	refresh(&now)
	lker.Unlock()
	r.WebClientinfos[len(r.WebClientinfos)-1].EndTimestamp = now.UnixNano()
	r.WebServerinfos[len(r.WebServerinfos)-1].EndTimestamp = now.UnixNano()
	r.GrpcClientinfos[len(r.GrpcClientinfos)-1].EndTimestamp = now.UnixNano()
	r.GrpcServerinfos[len(r.GrpcServerinfos)-1].EndTimestamp = now.UnixNano()
	r.CrpcClientinfos[len(r.CrpcClientinfos)-1].EndTimestamp = now.UnixNano()
	r.CrpcServerinfos[len(r.CrpcServerinfos)-1].EndTimestamp = now.UnixNano()
	for _, cinfo := range r.WebClientinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	for _, cinfo := range r.WebServerinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	for _, cinfo := range r.GrpcClientinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	for _, cinfo := range r.GrpcServerinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	for _, cinfo := range r.CrpcClientinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	for _, cinfo := range r.CrpcServerinfos {
		for _, peer := range cinfo.Callinfos {
			for _, path := range peer {
				path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.SuccessCount+path.ErrCount)
			}
		}
	}
	return r
}
func getT(data *[114]uint32, maxtimewaste uint64, totalcount uint32) (uint64, uint64, uint64) {
	if totalcount == 0 {
		return 0, 0, 0
	}
	var T50, T90, T99 uint64
	T50Count := uint32(float64(totalcount)*0.49) + 1
	T90Count := uint32(float64(totalcount)*0.9) + 1
	T99Count := uint32(float64(totalcount)*0.99) + 1
	var sum uint32
	var prefixtime uint64
	for index, count := range *data {
		var timepiece uint64
		switch {
		case index < 10:
			timepiece = uint64(time.Millisecond)
		case index < 28:
			timepiece = uint64(time.Millisecond) * 5
		case index < 73:
			timepiece = uint64(time.Millisecond) * 20
		case index < 113:
			timepiece = uint64(time.Millisecond) * 100
		default:
			timepiece = maxtimewaste - uint64(time.Second)*5
		}
		if sum+count >= T99Count && T99 == 0 {
			T99 = prefixtime + uint64(float64(timepiece)*(float64(T99Count-sum)/float64(count)))
		}
		if sum+count >= T90Count && T90 == 0 {
			T90 = prefixtime + uint64(float64(timepiece)*(float64(T90Count-sum)/float64(count)))
		}
		if sum+count >= T50Count && T50 == 0 {
			T50 = prefixtime + uint64(float64(timepiece)*(float64(T50Count-sum)/float64(count)))
		}
		if T99 != 0 && T90 != 0 && T50 != 0 {
			break
		}
		sum += count
		prefixtime += timepiece
	}
	return T50, T90, T99
}
