package mids

import (
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"
)

type ip struct {
	white     map[string]*struct{}
	whitemask map[uint64]int
	black     map[string]*struct{}
	blackmask map[uint64]int
}

var ipInstance *ip

func init() {
	ipInstance = &ip{
		white:     make(map[string]*struct{}),
		whitemask: make(map[uint64]int),
		black:     make(map[string]*struct{}),
		blackmask: make(map[uint64]int),
	}
}

// can only support ipv4
// can support ipv4 mask
func UpdateIpConfig(white []string, black []string) {
	w := make(map[string]*struct{})
	wm := make(map[uint64]int)
	b := make(map[string]*struct{})
	bm := make(map[uint64]int)
	for _, v := range white {
		if !CheckIpAndMask(v) {
			//skip illegal
			continue
		}
		if ipmask := strings.Split(v, "/"); len(ipmask) == 1 {
			w[v] = nil
		} else {
			//has mask
			mask, _ := strconv.Atoi(ipmask[1])
			self := big.NewInt(0).SetBytes(net.ParseIP(ipmask[0]).To4()).Uint64()
			wm[self] = mask
		}
	}
	for _, v := range black {
		if !CheckIpAndMask(v) {
			//skip illegal
			continue
		}
		if ipmask := strings.Split(v, "/"); len(ipmask) == 1 {
			b[v] = nil
		} else {
			//has mask
			mask, _ := strconv.Atoi(ipmask[1])
			self := big.NewInt(0).SetBytes(net.ParseIP(ipmask[0]).To4()).Uint64()
			bm[self] = mask
		}
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.white)), unsafe.Pointer(&w))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.whitemask)), unsafe.Pointer(&wm))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.black)), unsafe.Pointer(&b))
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.blackmask)), unsafe.Pointer(&bm))
}

func CheckIpAndMask(ip string) bool {
	if strings.Count(ip, "/") > 1 {
		return false
	}
	ipmask := strings.Split(ip, "/")
	if len(ipmask) == 2 {
		mask, e := strconv.Atoi(ipmask[1])
		if e != nil || mask > 32 || mask <= 0 {
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
func WhiteIP(ip string) bool {
	white := *(*map[string]*struct{})(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.white))))
	whitemask := *(*map[uint64]int)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.whitemask))))
	return checkip(white, whitemask, ip)
}

// true - in black ip list
// false - not in black ip list
func BlackIP(ip string) bool {
	black := *(*map[string]*struct{})(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.black))))
	blackmask := *(*map[uint64]int)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&ipInstance.blackmask))))
	return checkip(black, blackmask, ip)
}

// true - in
// false - not in
func checkip(nomask map[string]*struct{}, mask map[uint64]int, ip string) bool {
	if _, ok := nomask[ip]; ok {
		return true
	}
	for self, mask := range mask {
		target := big.NewInt(0).SetBytes(net.ParseIP(ip).To4()).Uint64()
		if (self>>(32-mask))<<(32-mask) == (target>>(32-mask))<<(32-mask) {
			return true
		}
	}
	return false
}
