package mids

import (
	"strings"
	"time"

	"github.com/chenjie199234/Corelib/container/ring"
)

type rate struct {
	grpc  map[string]*ring.Ring[int64] //key path
	crpc  map[string]*ring.Ring[int64] //key path
	get   map[string]*ring.Ring[int64] //key path
	post  map[string]*ring.Ring[int64] //key path
	put   map[string]*ring.Ring[int64] //key path
	patch map[string]*ring.Ring[int64] //key path
	del   map[string]*ring.Ring[int64] //key path
}

var rateinstance *rate

func init() {
	rateinstance = &rate{
		grpc:  make(map[string]*ring.Ring[int64]),
		crpc:  make(map[string]*ring.Ring[int64]),
		get:   make(map[string]*ring.Ring[int64]),
		post:  make(map[string]*ring.Ring[int64]),
		put:   make(map[string]*ring.Ring[int64]),
		patch: make(map[string]*ring.Ring[int64]),
		del:   make(map[string]*ring.Ring[int64]),
	}
}

type RateConfig struct {
	Path      string
	Method    []string //GRPC,CRPC,GET,POST,PUT,PATCH,DELETE
	MaxPerSec uint64   //0 means ban
}

func UpdateRateConfig(c []*RateConfig) {
	grpc := make(map[string]*ring.Ring[int64])  //key path
	crpc := make(map[string]*ring.Ring[int64])  //key path
	get := make(map[string]*ring.Ring[int64])   //key path
	post := make(map[string]*ring.Ring[int64])  //key path
	put := make(map[string]*ring.Ring[int64])   //key path
	patch := make(map[string]*ring.Ring[int64]) //key path
	del := make(map[string]*ring.Ring[int64])   //key path
	for _, cc := range c {
		r := ring.NewRing[int64](cc.MaxPerSec)
		for _, m := range cc.Method {
			switch strings.ToUpper(m) {
			case "GRPC":
				grpc[cc.Path] = r
			case "CRPC":
				crpc[cc.Path] = r
			case "GET":
				get[cc.Path] = r
			case "POST":
				post[cc.Path] = r
			case "PUT":
				put[cc.Path] = r
			case "PATCH":
				patch[cc.Path] = r
			case "DELETE":
				del[cc.Path] = r
			}
		}
	}
	rateinstance.grpc = grpc
	rateinstance.crpc = crpc
	rateinstance.get = get
	rateinstance.post = post
	rateinstance.put = put
	rateinstance.patch = patch
	rateinstance.del = del
}

func checkrate(buf *ring.Ring[int64]) bool {
	now := time.Now().UnixNano()
	for {
		if buf.Push(now) {
			return true
		}
		//buf full,try to pop
		if _, ok := buf.Pop(func(d int64) bool {
			if now-d >= time.Second.Nanoseconds() {
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
	buf, ok := rateinstance.grpc[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func CrpcRate(path string) bool {
	buf, ok := rateinstance.crpc[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func HttpGetRate(path string) bool {
	buf, ok := rateinstance.get[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func HttpPostRate(path string) bool {
	buf, ok := rateinstance.post[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func HttpPutRate(path string) bool {
	buf, ok := rateinstance.put[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func HttpPatchRate(path string) bool {
	buf, ok := rateinstance.patch[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
func HttpDelRate(path string) bool {
	buf, ok := rateinstance.del[path]
	if !ok {
		return true
	}
	return checkrate(buf)
}
