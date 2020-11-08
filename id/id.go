package id

import (
	"fmt"
	"sync/atomic"
	"time"
)

var offset uint64 = uint64(time.Date(2020, 5, 21, 13, 14, 0, 0, time.UTC).Unix())

var lasttime uint64
var serverid uint64
var rollback uint64

//64bit data
//00000000000000000000000000000000         000                000000               0          0000000000000000000000
//----32 bit timestamp(second)------3bit time rollback-----6bit serverid----1bit maxid lock-------22bit id----------
//-----can support 136 years--------rollback 8 times/s---can support 60 servers----------can make 4,000,000+ ids in one second
var base uint64

var inited int64

func New(sid uint64) {
	if atomic.SwapInt64(&inited, 1) == 1 {
		return
	}
	if sid <= 0 || sid >= 64 {
		panic(fmt.Sprintf("[ID.init]serviceid:%d range wrong,only support [1-63]", sid))
	}
	serverid = sid
	now := uint64(time.Now().Unix())
	templasttime := now - offset
	if now <= offset || templasttime > (1<<32-1) {
		panic("[ID.init]server time wrong")
	}
	lasttime = templasttime
	rollback = 1
	base = getlasttime() + getrollback() + getserverid()
	go func() {
		tker := time.NewTicker(200 * time.Millisecond)
		for {
			<-tker.C
			now = uint64(time.Now().Unix())
			templasttime = now - offset
			if now <= offset || templasttime > (1<<32-1) {
				panic("[ID.init]server time wrong")
			}
			if lasttime != templasttime {
				if templasttime < lasttime {
					rollback++
					fmt.Printf("[ID.init]server time rollback,old time:%d current time:%d rollback:%d\n", lasttime, templasttime, rollback&7)
				}
				lasttime = templasttime
				base = getlasttime() + getrollback() + getserverid()
			}
		}
	}()
}
func getlasttime() uint64 {
	return lasttime << 32
}
func getrollback() uint64 {
	return (rollback & 7) << 29
}
func getserverid() uint64 {
	return serverid << 23
}

var ERRMAX = fmt.Errorf("[ID.GetID]Max id was used up in this second")

func GetID() (uint64, error) {
	if (base & (1 << 22)) > 0 {
		return 0, ERRMAX
	}
	newid := atomic.AddUint64(&base, 1)
	if (newid & (1 << 22)) > 0 {
		return 0, ERRMAX
	}
	return newid, nil
}
