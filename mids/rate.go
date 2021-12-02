package mids

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type rate struct {
	grpc  map[string]*counter //key path
	crpc  map[string]*counter //key path
	get   map[string]*counter //key path
	post  map[string]*counter //key path
	put   map[string]*counter //key path
	patch map[string]*counter //key path
	del   map[string]*counter //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{
		grpc:  make(map[string]*counter),
		crpc:  make(map[string]*counter),
		get:   make(map[string]*counter),
		post:  make(map[string]*counter),
		put:   make(map[string]*counter),
		patch: make(map[string]*counter),
		del:   make(map[string]*counter),
	}
}

type RateConfig struct {
	Path      string
	Method    []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec int      //0 means ban
}

func UpdateRateConfig(c []*RateConfig) {
	grpc := make(map[string]*counter)  //key path
	crpc := make(map[string]*counter)  //key path
	get := make(map[string]*counter)   //key path
	post := make(map[string]*counter)  //key path
	put := make(map[string]*counter)   //key path
	patch := make(map[string]*counter) //key path
	del := make(map[string]*counter)   //key path
	for _, cc := range c {
		tmp := &counter{
			calls: make([]int64, cc.MaxPerSec),
		}
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = tmp
			case "CRPC":
				crpc[cc.Path] = tmp
			case "GET":
				get[cc.Path] = tmp
			case "POST":
				post[cc.Path] = tmp
			case "PUT":
				put[cc.Path] = tmp
			case "PATCH":
				patch[cc.Path] = tmp
			case "DELETE":
				del[cc.Path] = tmp
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

type counter struct {
	sync.Mutex
	head, tail int
	calls      []int64
}

func (c *counter) check() (pass bool) {
	c.Lock()
	defer c.Unlock()
	if len(c.calls) == 0 {
		pass = false
		return
	}
	now := time.Now()
	newtail := c.tail + 1
	if newtail >= len(c.calls) {
		newtail = 0
	}
	if newtail == c.head {
		//full try to pop
		if now.UnixNano()-c.calls[c.head] >= time.Second.Nanoseconds() {
			c.head++
			if c.head >= len(c.calls) {
				c.head = 0
			}
		} else {
			pass = false
			return
		}
	}
	c.calls[newtail] = now.UnixNano()
	c.tail = newtail
	pass = true
	return
}

func GrpcRate(path string) (pass bool) {
	grpc := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.grpc))))
	c, ok := grpc[path]
	if !ok {
		pass = true
		return
	}
	return c.check()
}
func CrpcRate(path string) (pass bool) {
	crpc := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.crpc))))
	c, ok := crpc[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
func HttpGetRate(path string) (pass bool) {
	get := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.get))))
	c, ok := get[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
func HttpPostRate(path string) (pass bool) {
	post := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.post))))
	c, ok := post[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
func HttpPutRate(path string) (pass bool) {
	put := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.put))))
	c, ok := put[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
func HttpPatchRate(path string) (pass bool) {
	patch := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.patch))))
	c, ok := patch[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
func HttpDelRate(path string) (pass bool) {
	del := *(*map[string]*counter)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&rateinstance.del))))
	c, ok := del[path]
	if !ok {
		pass = true
		return
	}
	pass = c.check()
	return
}
