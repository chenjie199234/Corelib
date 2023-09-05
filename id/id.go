package id

import (
	"errors"
	"sync/atomic"
	"time"
)

// 2023-05-21 13:14:00(UTC)
const offset uint64 = 1684646040

var lasttime uint64
var serverid uint64
var rollback uint64

// 64bit data
// 00000000000000000000000000000000         000000                000                    00000000000000000000000
// ----32 bit timestamp(second)----------6bit rollback-----3bit serverid------------------------23bit id-------
// -----can support 136 years---------rollback 60 seconds---can support 8 servers-----can make 8,000,000+ ids in one second per server
var base uint64

var inited int64

// thread safe
func New(sid uint64) {
	if atomic.SwapInt64(&inited, 1) == 1 {
		return
	}
	if sid < 0 || sid > 7 {
		panic("[ID.init]serviceid range wrong,only support [0-7]")
	}
	serverid = sid
	now := uint64(time.Now().Unix())
	templasttime := now - offset
	if now < offset || templasttime > (1<<32-1) {
		panic("[ID.init]server time wrong")
	}
	lasttime = templasttime
	rollback = 0
	base = getlasttime() + getrollback() + getserverid()
	go func() {
		tker := time.NewTicker(200 * time.Millisecond)
		for {
			<-tker.C
			now = uint64(time.Now().Unix())
			templasttime = now - offset
			if now < offset || templasttime > (1<<32-1) {
				panic("[ID.init]server time wrong")
			}
			if templasttime > lasttime {
				//refresh base
				lasttime = templasttime
				rollback = 0
				atomic.StoreUint64(&base, getlasttime()+getrollback()+getserverid())
			} else if templasttime < lasttime {
				//rollback
				rollback++
				if rollback > 63 {
					panic("[ID.init] server time rollback more then 60s")
				}
				atomic.StoreUint64(&base, getlasttime()+getrollback()+getserverid())
			}
		}
	}()
}
func getlasttime() uint64 {
	return lasttime << 32
}
func getrollback() uint64 {
	return (rollback & 63) << 26
}
func getserverid() uint64 {
	return serverid << 23
}

const serveridmask uint64 = uint64(7) << 23

func checkserverid(id uint64) bool {
	if ((id & serveridmask) >> 23) == serverid {
		return true
	}
	return false
}

var ERRMAX = errors.New("[ID.GetID] no more ids in this second")

func GetID() (uint64, error) {
	_, end, e := GetIDs(1)
	return end, e
}

// range is [start,end],including start and end,if delta is 1,start = end
func GetIDs(delta uint16) (start uint64, end uint64, e error) {
	if delta == 0 {
		delta += 1
	}
	for {
		oldbase := atomic.LoadUint64(&base)
		if !checkserverid(oldbase + uint64(delta)) {
			return 0, 0, ERRMAX
		}
		if !atomic.CompareAndSwapUint64(&base, oldbase, oldbase+uint64(delta)) {
			continue
		}
		return oldbase + 1, oldbase + uint64(delta), nil
	}
}
