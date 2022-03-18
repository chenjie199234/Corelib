package mids

import (
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/container/ring"
)

type rate struct {
	grpc  map[string]*ring.Ring //key path
	crpc  map[string]*ring.Ring //key path
	get   map[string]*ring.Ring //key path
	post  map[string]*ring.Ring //key path
	put   map[string]*ring.Ring //key path
	patch map[string]*ring.Ring //key path
	del   map[string]*ring.Ring //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{
		grpc:  make(map[string]*ring.Ring),
		crpc:  make(map[string]*ring.Ring),
		get:   make(map[string]*ring.Ring),
		post:  make(map[string]*ring.Ring),
		put:   make(map[string]*ring.Ring),
		patch: make(map[string]*ring.Ring),
		del:   make(map[string]*ring.Ring),
	}
}

type RateConfig struct {
	Path      string
	Method    []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec uint64   //0 means ban
}

func UpdateRateConfig(c []*RateConfig) {
	grpc := make(map[string]*ring.Ring)  //key path
	crpc := make(map[string]*ring.Ring)  //key path
	get := make(map[string]*ring.Ring)   //key path
	post := make(map[string]*ring.Ring)  //key path
	put := make(map[string]*ring.Ring)   //key path
	patch := make(map[string]*ring.Ring) //key path
	del := make(map[string]*ring.Ring)   //key path
	for _, cc := range c {
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "CRPC":
				crpc[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "GET":
				get[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "POST":
				post[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "PUT":
				put[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "PATCH":
				patch[cc.Path] = ring.NewRing(cc.MaxPerSec)
			case "DELETE":
				del[cc.Path] = ring.NewRing(cc.MaxPerSec)
			}
		}
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.grpc)), unsafe.Pointer(&grpc))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.crpc)), unsafe.Pointer(&crpc))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.get)), unsafe.Pointer(&get))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.post)), unsafe.Pointer(&post))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.put)), unsafe.Pointer(&put))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.patch)), unsafe.Pointer(&patch))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.del)), unsafe.Pointer(&del))
}

func check(buf *ring.Ring) bool {
	now := time.Now().UnixNano()
	for {
		if buf.Push(unsafe.Pointer(&now)) {
			return true
		}
		//buf full,try to pop
		if _, ok := buf.Pop(func(d unsafe.Pointer) bool {
			if now-*(*int64)(d) >= time.Second.Nanoseconds() {
				return true
			}
			return false
		}); !ok {
			//can't push and can't pop,buf is still full
			break
		}
	}
	return false
}

func GrpcRate(path string) bool {
	grpc := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.grpc))))
	buf, ok := grpc[path]
	if !ok {
		return true
	}
	return check(buf)
}
func CrpcRate(path string) bool {
	crpc := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.crpc))))
	buf, ok := crpc[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpGetRate(path string) bool {
	get := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.get))))
	buf, ok := get[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPostRate(path string) bool {
	post := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.post))))
	buf, ok := post[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPutRate(path string) bool {
	put := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.put))))
	buf, ok := put[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPatchRate(path string) bool {
	patch := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.patch))))
	buf, ok := patch[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpDelRate(path string) bool {
	del := *(*map[string]*ring.Ring)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.del))))
	buf, ok := del[path]
	if !ok {
		return true
	}
	return check(buf)
}
