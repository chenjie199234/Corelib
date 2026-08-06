package mids

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

type ip struct {
	white atomic.Pointer[whiteblack]
	black atomic.Pointer[whiteblack]
}
type whiteblack struct {
	nomask  map[string]struct{} //key ipv4
	hasmask map[uint32]int      //key ipv4,value subnet mask
}

var ipInstance *ip

func init() {
	ipInstance = &ip{}
}

// can only support ipv4
// can support ipv4 subnet mask
func UpdateIpConfig(white []string, black []string) {
	w := make(map[string]struct{})
	wm := make(map[uint32]int)
	b := make(map[string]struct{})
	bm := make(map[uint32]int)
	for _, v := range white {
		if !CheckIpAndMask(v) {
			//skip illegal
			continue
		}
		if ipmask := strings.Split(v, "/"); len(ipmask) == 1 {
			w[v] = struct{}{}
		} else {
			//has mask
			mask, _ := strconv.Atoi(ipmask[1])
			if mask == 0 {
				w[v] = struct{}{}
			} else {
				self := binary.BigEndian.Uint32(net.ParseIP(ipmask[0]).To4())
				wm[self] = mask
			}
		}
	}
	for _, v := range black {
		if !CheckIpAndMask(v) {
			//skip illegal
			continue
		}
		if ipmask := strings.Split(v, "/"); len(ipmask) == 1 {
			b[v] = struct{}{}
		} else {
			//has mask
			mask, _ := strconv.Atoi(ipmask[1])
			if mask == 0 {
				b[v] = struct{}{}
			} else {
				self := binary.BigEndian.Uint32(net.ParseIP(ipmask[0]).To4())
				bm[self] = mask
			}
		}
	}
	ipInstance.white.Store(&whiteblack{
		nomask:  w,
		hasmask: wm,
	})
	ipInstance.black.Store(&whiteblack{
		nomask:  b,
		hasmask: bm,
	})
}

func CheckIpAndMask(ip string) bool {
	if strings.Count(ip, "/") > 1 {
		return false
	}
	ipmask := strings.Split(ip, "/")
	if len(ipmask) == 2 {
		mask, e := strconv.Atoi(ipmask[1])
		if e != nil || mask > 32 || mask < 0 {
			return false
		}
	}
	pieces := strings.Split(ipmask[0], ".")
	if len(pieces) != 4 {
		return false
	}
	for _, piece := range pieces {
		n, e := strconv.Atoi(piece)
		if e != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// true - in white ip list
// false - not in white ip list
func WhiteIP(ip string) (pass bool) {
	w := ipInstance.white.Load()
	if w == nil {
		//require white ip check,but the config missing
		return false
	}
	if _, ok := w.nomask[ip]; ok {
		return true
	}
	tmp := net.ParseIP(ip).To4()
	if tmp == nil {
		//illegal ip
		return false
	}
	target := binary.BigEndian.Uint32(tmp)
	for self, mask := range w.hasmask {
		if (self>>(32-mask))<<(32-mask) == (target>>(32-mask))<<(32-mask) {
			return true
		}
	}
	return false
}

// true - in black ip list
// false - not in black ip list
func BlackIP(ip string) (block bool) {
	b := ipInstance.black.Load()
	if b == nil {
		//require black ip check,but the config missing
		return true
	}
	if _, ok := b.nomask[ip]; ok {
		return true
	}
	tmp := net.ParseIP(ip).To4()
	if tmp == nil {
		//illegal ip
		return true
	}
	target := binary.BigEndian.Uint32(tmp)
	for self, mask := range b.hasmask {
		if (self>>(32-mask))<<(32-mask) == (target>>(32-mask))<<(32-mask) {
			return true
		}
	}
	return false
}
