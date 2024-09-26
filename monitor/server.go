package monitor

import (
	"sync"

	"github.com/chenjie199234/Corelib/cerror"
)

var serverinited bool

var wslker sync.Mutex
var gslker sync.Mutex
var cslker sync.Mutex
var webServerinfos map[string]map[string]*pathinfo  //first key peername,second key path
var grpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path
var crpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path

func initserver() {
	serverinited = true
	webServerinfos = make(map[string]map[string]*pathinfo)
	grpcServerinfos = make(map[string]map[string]*pathinfo)
	crpcServerinfos = make(map[string]map[string]*pathinfo)
}
func serverCollect() (web, grpc, crpc map[string]map[string]*pathinfo) {
	wslker.Lock()
	gslker.Lock()
	cslker.Lock()
	defer func() {
		initserver()
		wslker.Unlock()
		gslker.Unlock()
		cslker.Unlock()
	}()
	return webServerinfos, grpcServerinfos, crpcServerinfos
}
func WebServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !serverinited {
		return
	}
	recordpath := method + ":" + path
	wslker.Lock()
	peer, ok := webServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		webServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	wslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.Convert(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}

func GrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !serverinited {
		return
	}
	recordpath := method + ":" + path
	gslker.Lock()
	peer, ok := grpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		grpcServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	gslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.Convert(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !serverinited {
		return
	}
	recordpath := method + ":" + path
	cslker.Lock()
	peer, ok := crpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		crpcServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	cslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.Convert(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
