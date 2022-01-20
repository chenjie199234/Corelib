package monitor

import (
	"runtime"
	"sync"
)

type monitor struct {
	Routinenum  int
	Threadnum   int
	Heapnum     int
	wclker      *sync.RWMutex
	Webclient   map[string]*callinfo //key peername
	wslker      *sync.RWMutex
	Webserver   map[string]*callinfo //key peername
	gclker      *sync.RWMutex
	Cgrpcclient map[string]*callinfo //key peername
	gslker      *sync.RWMutex
	Cgrpcserver map[string]*callinfo //key peername
	cclker      *sync.RWMutex
	Crpcclient  map[string]*callinfo //key peername
	cslker      *sync.RWMutex
	Crpcserver  map[string]*callinfo //key peername
}
type callinfo struct {
	sync.RWMutex
	Paths map[string]*pathinfo //key path
}
type pathinfo struct {
}

var m *monitor

func init() {
	m = &monitor{
		wclker:      &sync.RWMutex{},
		Webclient:   make(map[string]*callinfo),
		wslker:      &sync.RWMutex{},
		Webserver:   make(map[string]*callinfo),
		gclker:      &sync.RWMutex{},
		Cgrpcclient: make(map[string]*callinfo),
		gslker:      &sync.RWMutex{},
		Cgrpcserver: make(map[string]*callinfo),
		cclker:      &sync.RWMutex{},
		Crpcclient:  make(map[string]*callinfo),
		cslker:      &sync.RWMutex{},
		Crpcserver:  make(map[string]*callinfo),
	}
}
func WebClient() {
}
func WebServer() {
}
func CgrpcClient() {
}
func CgrpcServer() {
}
func CrpcClient() {
}
func CrpcServer() {
}
func getinfo() {
	m.Routinenum = runtime.NumGoroutine()
	m.Threadnum, _ = runtime.ThreadCreateProfile(nil)
	runtime.MemProfile()
	//runtime.ReadMemStats()
}
