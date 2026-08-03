package id

import (
	"errors"
	"sync/atomic"
	"time"
)

// 2026-05-21 13:14:00(UTC)
const offset uint64 = 1779369240
const tshift int = 32 //timestamp shift
const rshift int = 30 //rollback shift
const sshift int = 14 //sid shift
const maxid uint64 = (1 << sshift) - 1

type IDGenerator struct {
	// 64bit data
	// 00000000000000000000000000000000           00               0000000000000000                     00000000000000
	// ----32 bit timestamp(second)---------2 bit rollback-----------16 bit sid---------------------------14 bit id---
	// -----can support 136 years----------max 3 rollbacks-----can support subnet mask /16----can make 16384 ids in one second per server
	base       uint64
	compensate uint64
	last       uint64
	rollback   uint8
	sid        uint16
}

// thread safe
// IDGenerator can only create 16384 ids in one second per sid
// sid can use ipv4's right 16 bit with subnet mask /16
// example:
// var ipv4 uint32 = GetIpv4()
// sid := (ipv4<<16)>>16
func New(sid uint16) *IDGenerator {
	now := uint64(time.Now().Unix())
	if now < offset || now-offset > (1<<32-1) {
		panic("[ID.New] host server time wrong")
	}
	id := &IDGenerator{
		sid:  sid,
		last: now - offset,
	}
	atomic.StoreUint64(&id.base, id.last<<tshift+uint64(id.rollback)<<rshift+uint64(id.sid)<<sshift)
	go func() {
		tker := time.NewTicker(200 * time.Millisecond)
		for {
			t := <-tker.C
			if uint64(t.Unix()) < offset || uint64(t.Unix())-offset > (1<<32-1) {
				panic("[ID.New.go] host server time wrong")
			}
			if uint64(t.Unix())-offset > id.last {
				//normal or rollback fixed
				id.last = uint64(t.Unix()) - offset
				id.compensate = 0
				id.rollback = 0
				atomic.StoreUint64(&id.base, id.last<<tshift+uint64(id.rollback)<<rshift+uint64(id.sid)<<sshift)
			} else if id.compensate == 0 {
				if uint64(t.Unix())-offset < id.last {
					//rollback happened,keep the last and update compensate and increase the rollback
					id.compensate = id.last - (uint64(t.Unix()) - offset)
					id.rollback++
					if id.rollback >= 4 {
						panic("[ID.New.go] host server time wrong,rollback >= 4 times")
					}
					atomic.StoreUint64(&id.base, id.last<<tshift+uint64(id.rollback)<<rshift+uint64(id.sid)<<sshift)
				}
			} else if uint64(t.Unix())+id.compensate-offset > id.last {
				//work on rollback status
				id.last = uint64(t.Unix()) + id.compensate - offset
				atomic.StoreUint64(&id.base, id.last<<tshift+uint64(id.rollback)<<rshift+uint64(id.sid)<<sshift)
			} else if uint64(t.Unix())+id.compensate-offset < id.last {
				//rollback happened again,keep the last and update compensate and increase the rollback
				id.compensate += id.last - (uint64(t.Unix()) + id.compensate - offset)
				id.rollback++
				if id.rollback >= 4 {
					panic("[ID.New.go] host server time wrong,rollback >= 4 times")
				}
				atomic.StoreUint64(&id.base, id.last<<tshift+uint64(id.rollback)<<rshift+uint64(id.sid)<<sshift)
			}
		}
	}()
	return id
}

var ERRMAX = errors.New("[ID.GetIDs] no more ids in this second")

func (id *IDGenerator) GetID() (uint64, error) {
	_, end, e := id.GetIDs(1)
	return end, e
}

// range is [start,end],including start and end,if delta is 1,start = end
func (id *IDGenerator) GetIDs(delta uint8) (start uint64, end uint64, e error) {
	if delta == 0 {
		panic("[ID.GetIDs] require id num must > 0")
	}
	for {
		oldbase := atomic.LoadUint64(&id.base)
		curid := (oldbase << (64 - sshift)) >> (64 - sshift)
		if curid+uint64(delta) > maxid {
			return 0, 0, ERRMAX
		}
		if !atomic.CompareAndSwapUint64(&id.base, oldbase, oldbase+uint64(delta)) {
			continue
		}
		return oldbase, oldbase + uint64(delta) - 1, nil
	}
}
