package id

import (
	"errors"
	"sync/atomic"
	"time"
)

//2020-05-21 13:14:00
const offset uint64 = 1590066840

var lasttime uint64
var serverid uint64
var rollback uint64

//64bit data
//00000000000000000000000000000000         000000                000                    00000000000000000000000
//----32 bit timestamp(second)----------6bit rollback-----3bit serverid------------------------23bit id-------
//-----can support 136 years---------rollback 60 seconds---can support 8 servers-----can make 8,000,000+ ids in one second per server
var base uint64

var inited int64

//thread safe
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
				base = getlasttime() + getrollback() + getserverid()
			} else if templasttime < lasttime {
				//rollback
				rollback++
				if rollback > 63 {
					panic("[ID.init] server time rollback more then 60s")
				}
				base = getlasttime() + getrollback() + getserverid()
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

const mask uint64 = uint64(63) << 23

func checkserverid(id uint64) bool {
	if ((id & mask) >> 23) == serverid {
		return true
	}
	return false
}

var ERRMAX = errors.New("[ID.GetID]Max id was used up in this second")

func GetID() (uint64, error) {
	if !checkserverid(base) {
		return 0, ERRMAX
	}
	newid := atomic.AddUint64(&base, 1)
	if !checkserverid(newid) {
		return 0, ERRMAX
	}
	return newid, nil
}

var ERRMAXONCE = errors.New("[ID.GetID]Too many ids required once")

func GetIDs(delta uint64) (start uint64, end uint64, e error) {
	if delta > 5000 {
		return 0, 0, ERRMAXONCE
	}
	if !checkserverid(base) {
		return 0, 0, ERRMAX
	}
	newid := atomic.AddUint64(&base, delta)
	if !checkserverid(newid) {
		return 0, 0, ERRMAX
	}
	start = newid - delta
	end = newid
	return
}
