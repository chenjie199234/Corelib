package mids

import (
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/container/ringbuf"
)

type rate struct {
	grpc  map[string]*ringbuf.CasRingBuf //key path
	crpc  map[string]*ringbuf.CasRingBuf //key path
	get   map[string]*ringbuf.CasRingBuf //key path
	post  map[string]*ringbuf.CasRingBuf //key path
	put   map[string]*ringbuf.CasRingBuf //key path
	patch map[string]*ringbuf.CasRingBuf //key path
	del   map[string]*ringbuf.CasRingBuf //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{
		grpc:  make(map[string]*ringbuf.CasRingBuf),
		crpc:  make(map[string]*ringbuf.CasRingBuf),
		get:   make(map[string]*ringbuf.CasRingBuf),
		post:  make(map[string]*ringbuf.CasRingBuf),
		put:   make(map[string]*ringbuf.CasRingBuf),
		patch: make(map[string]*ringbuf.CasRingBuf),
		del:   make(map[string]*ringbuf.CasRingBuf),
	}
}

type RateConfig struct {
	Path      string
	Method    []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec uint64   //0 means ban
}

func UpdateRateConfig(c []*RateConfig) {
	grpc := make(map[string]*ringbuf.CasRingBuf)  //key path
	crpc := make(map[string]*ringbuf.CasRingBuf)  //key path
	get := make(map[string]*ringbuf.CasRingBuf)   //key path
	post := make(map[string]*ringbuf.CasRingBuf)  //key path
	put := make(map[string]*ringbuf.CasRingBuf)   //key path
	patch := make(map[string]*ringbuf.CasRingBuf) //key path
	del := make(map[string]*ringbuf.CasRingBuf)   //key path
	for _, cc := range c {
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "CRPC":
				crpc[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "GET":
				get[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "POST":
				post[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "PUT":
				put[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "PATCH":
				patch[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
			case "DELETE":
				del[cc.Path] = ringbuf.NewCasRingBuf(cc.MaxPerSec)
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

func check(buf *ringbuf.CasRingBuf) bool {
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
	grpc := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.grpc))))
	buf, ok := grpc[path]
	if !ok {
		return true
	}
	return check(buf)
}
func CrpcRate(path string) bool {
	crpc := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.crpc))))
	buf, ok := crpc[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpGetRate(path string) bool {
	get := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.get))))
	buf, ok := get[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPostRate(path string) bool {
	post := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.post))))
	buf, ok := post[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPutRate(path string) bool {
	put := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.put))))
	buf, ok := put[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpPatchRate(path string) bool {
	patch := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.patch))))
	buf, ok := patch[path]
	if !ok {
		return true
	}
	return check(buf)
}
func HttpDelRate(path string) bool {
	del := *(*map[string]*ringbuf.CasRingBuf)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.del))))
	buf, ok := del[path]
	if !ok {
		return true
	}
	return check(buf)
}
