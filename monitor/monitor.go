package monitor

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	//"github.com/chenjie199234/Corelib/bufpool"
	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
)

var m monitor
var refresher *time.Timer

var wclker sync.Mutex
var wslker sync.Mutex
var gclker sync.Mutex
var gslker sync.Mutex
var cclker sync.Mutex
var cslker sync.Mutex

type monitor struct {
	Sysinfos        *sysinfo
	WebClientinfos  map[string]map[string]*pathinfo //first key peername,second key path
	WebServerinfos  map[string]map[string]*pathinfo //first key peername,second key path
	GrpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path
	GrpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path
	CrpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path
	CrpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path
}
type sysinfo struct {
	RoutineNum  int
	ThreadNum   int
	HeapobjNum  int
	GctimeWaste int
}
type pathinfo struct {
	TotalCount   uint32
	ErrCodeCount map[int32]uint32
	T50          uint64      //nano second
	T90          uint64      //nano second
	T99          uint64      //nano second
	maxTimewaste uint64      //nano second
	timewaste    [114]uint32 //value:count,index:0-9(0ms-10ms) each 1ms,10-27(10ms-100ms) each 5ms,index:28-72(100ms-1s) each 20ms,index:73-112(1s-5s) each 100ms,index:113 more then 5s
	lker         *sync.Mutex
}

func init() {
	if str := os.Getenv("MONITOR"); str == "" || str == "<MONITOR>" {
		log.Warning(nil, "[monitor] env MONITOR missing,monitor closed")
		return
	} else if n, e := strconv.Atoi(str); e != nil || n != 0 && n != 1 {
		log.Warning(nil, "[monitor] env MONITOR format error,monitor closed")
		return
	} else if n == 0 {
		log.Warning(nil, "[monitor] env MONITOR is 0,monitor closed")
		return
	}
	refresh()
	go func() {
		<-refresher.C
		refresh()
	}()
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			tmpm := getMonitorInfo()
			if tmpm == nil {
				return
			}
			//buf := bufpool.GetBuffer()
			//for _, sysinfo := range tmpm.Sysinfos {

			//}
			//for _, gcinfo := range tmpm.GCinfos {

			//}
			//for _, webcinfo := range tmpm.WebClientinfos {

			//}
			//for _, websinfo := range tmpm.WebServerinfos {

			//}
			//for _, grpccinfo := range tmpm.GrpcClientinfos {

			//}
			//for _, grpcsinfo := range tmpm.GrpcServerinfos {

			//}
			//for _, crpccinfo := range tmpm.CrpcClientinfos {

			//}
			//for _, crpcsinfo := range tmpm.CrpcServerinfos {

			//}
		})
		http.ListenAndServe(":6060", nil)
	}()
}
func refresh() {
	m.WebClientinfos = make(map[string]map[string]*pathinfo)
	m.WebServerinfos = make(map[string]map[string]*pathinfo)
	m.GrpcClientinfos = make(map[string]map[string]*pathinfo)
	m.GrpcServerinfos = make(map[string]map[string]*pathinfo)
	m.CrpcClientinfos = make(map[string]map[string]*pathinfo)
	m.CrpcServerinfos = make(map[string]map[string]*pathinfo)
	if refresher == nil {
		refresher = time.NewTimer(time.Minute*2 + time.Second)
	} else {
		refresher.Reset(time.Minute*2 + time.Second)
		for len(refresher.C) > 0 {
			<-refresher.C
		}
	}
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
	wclker.Lock()
	defer wclker.Unlock()
	peer, ok := m.WebClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.WebClientinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func WebServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	wslker.Lock()
	defer wslker.Unlock()
	peer, ok := m.WebServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.WebServerinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func GrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	gclker.Lock()
	defer gclker.Unlock()
	peer, ok := m.GrpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.GrpcClientinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func GrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	gslker.Lock()
	defer gslker.Unlock()
	peer, ok := m.GrpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.GrpcServerinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	cclker.Lock()
	defer cclker.Unlock()
	peer, ok := m.CrpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.CrpcClientinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	recordpath := method + ":" + path
	cslker.Lock()
	defer cslker.Unlock()
	peer, ok := m.CrpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.CrpcServerinfos[peername] = peer
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
	atomic.AddUint32(&pinfo.TotalCount, 1)
	//error
	ee := cerror.ConvertStdError(e)
	pinfo.lker.Lock()
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}

var lastgcindex uint32

func getMonitorInfo() *monitor {
	wclker.Lock()
	wslker.Lock()
	gclker.Lock()
	gslker.Lock()
	cclker.Lock()
	cslker.Lock()
	r := &monitor{
		Sysinfos:        &sysinfo{},
		WebClientinfos:  m.WebClientinfos,
		WebServerinfos:  m.WebServerinfos,
		GrpcClientinfos: m.GrpcClientinfos,
		GrpcServerinfos: m.GrpcServerinfos,
		CrpcClientinfos: m.CrpcClientinfos,
		CrpcServerinfos: m.CrpcServerinfos,
	}
	refresh()
	wclker.Unlock()
	wslker.Unlock()
	gclker.Unlock()
	gslker.Unlock()
	cclker.Unlock()
	cslker.Unlock()
	for _, peer := range r.WebClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.WebServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.GrpcClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.GrpcServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.CrpcClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.CrpcServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	r.Sysinfos.RoutineNum = runtime.NumGoroutine()
	r.Sysinfos.ThreadNum, _ = runtime.ThreadCreateProfile(nil)
	meminfo := &runtime.MemStats{}
	runtime.ReadMemStats(meminfo)
	r.Sysinfos.HeapobjNum = int(meminfo.HeapObjects)
	if meminfo.NumGC > lastgcindex+256 {
		lastgcindex = meminfo.NumGC - 256
	}
	for lastgcindex < meminfo.NumGC {
		r.Sysinfos.GctimeWaste += int(meminfo.PauseNs[lastgcindex%256])
		lastgcindex++
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
			if maxtimewaste-prefixtime >= uint64(time.Millisecond) {
				timepiece = uint64(time.Millisecond)
			} else {
				timepiece = maxtimewaste - prefixtime
			}
		case index < 28:
			if maxtimewaste-prefixtime >= uint64(time.Millisecond)*5 {
				timepiece = uint64(time.Millisecond) * 5
			} else {
				timepiece = maxtimewaste - prefixtime
			}
		case index < 73:
			if maxtimewaste-prefixtime >= uint64(time.Millisecond)*20 {
				timepiece = uint64(time.Millisecond) * 20
			} else {
				timepiece = maxtimewaste - prefixtime
			}
		case index < 113:
			if maxtimewaste-prefixtime >= uint64(time.Millisecond)*100 {
				timepiece = uint64(time.Millisecond) * 100
			} else {
				timepiece = maxtimewaste - prefixtime
			}
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
