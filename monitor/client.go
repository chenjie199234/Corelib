package monitor

import (
	"sync"

	"github.com/chenjie199234/Corelib/cerror"
)

var clientinited bool

var wclker sync.Mutex
var gclker sync.Mutex
var cclker sync.Mutex
var webClientinfos map[string]map[string]*pathinfo  //first key peername,second key path
var grpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path
var crpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path

func initclient() {
	clientinited = true
	webClientinfos = make(map[string]map[string]*pathinfo)
	grpcClientinfos = make(map[string]map[string]*pathinfo)
	crpcClientinfos = make(map[string]map[string]*pathinfo)
}
func clientCollect() (web, grpc, crpc map[string]map[string]*pathinfo) {
	wclker.Lock()
	gclker.Lock()
	cclker.Lock()
	defer func() {
		initclient()
		wclker.Unlock()
		gclker.Unlock()
		cclker.Unlock()
	}()
	return webClientinfos, grpcClientinfos, crpcClientinfos
}
func WebClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !clientinited {
		return
	}
	recordpath := method + ":" + path
	wclker.Lock()
	peer, ok := webClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		webClientinfos[peername] = peer
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
	wclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func GrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !clientinited {
		return
	}
	recordpath := method + ":" + path
	gclker.Lock()
	peer, ok := grpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		grpcClientinfos[peername] = peer
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
	gclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !clientinited {
		return
	}
	recordpath := method + ":" + path
	cclker.Lock()
	peer, ok := crpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		crpcClientinfos[peername] = peer
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
	cclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
