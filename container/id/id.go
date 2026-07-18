package id

import (
	"errors"
	"sync/atomic"
	"time"
)

// 2026-05-21 13:14:00(UTC)
const offset uint64 = 1779369240

type IDGenerator struct {
	// 64bit data
	// 00000000000000000000000000000000         000000                  00                        000000000000000000000000
	// ----32 bit timestamp(second)---------6 bit rollback-----------2 bit sid---------------------------24 bit id--------
	// -----can support 136 years----------max 63 rollbacks---can support 4 servers-----can make 16,000,000+ ids in one second per server
	base       uint64
	compensate uint64
	last       uint64
	rollback   uint8
	sid        uint8
}

// thread safe
func New(sid uint8) *IDGenerator {
	if sid >= 4 {
		panic("[ID.New] server id must be in [0,3]")
	}
	now := uint64(time.Now().Unix())
	if now < offset || now-offset > (1<<32-1) {
		panic("[ID.New] host server time wrong")
	}
	id := &IDGenerator{
		sid:  sid,
		last: now - offset,
	}
	id.base = id.last<<32 + uint64(id.rollback)<<26 + uint64(id.sid)<<24
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
				atomic.StoreUint64(&id.base, id.last<<32+uint64(id.rollback)<<26+uint64(id.sid)<<24)
			} else if id.compensate == 0 {
				if uint64(t.Unix())-offset < id.last {
					//rollback happened,keep the last and update compensate and increase the rollback
					id.compensate = id.last - (uint64(t.Unix()) - offset)
					id.rollback++
					if id.rollback >= 64 {
						panic("[ID.New.go] host server time wrong,rollback >= 64 times")
					}
					atomic.StoreUint64(&id.base, id.last<<32+uint64(id.rollback)<<26+uint64(id.sid)<<24)
				}
			} else if uint64(t.Unix())+id.compensate-offset > id.last {
				//work on rollback status
				id.last = uint64(t.Unix()) + id.compensate - offset
				atomic.StoreUint64(&id.base, id.last<<32+uint64(id.rollback)<<26+uint64(id.sid)<<24)
			} else if uint64(t.Unix())+id.compensate-offset < id.last {
				//rollback happened again,keep the last and update compensate and increase the rollback
				id.compensate += id.last - (uint64(t.Unix()) + id.compensate - offset)
				id.rollback++
				if id.rollback >= 64 {
					panic("[ID.New.go] host server time wrong,rollback >= 64 times")
				}
				atomic.StoreUint64(&id.base, id.last<<32+uint64(id.rollback)<<26+uint64(id.sid)<<24)
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
func (id *IDGenerator) GetIDs(delta uint16) (start uint64, end uint64, e error) {
	if delta == 0 {
		panic("[ID.GetIDs] require id num must > 0")
	}
	for {
		oldbase := atomic.LoadUint64(&id.base)
		if (oldbase<<40)>>40+uint64(delta) > (1<<24)-1 {
			return 0, 0, ERRMAX
		}
		if !atomic.CompareAndSwapUint64(&id.base, oldbase, oldbase+uint64(delta)) {
			continue
		}
		return oldbase + 1, oldbase + uint64(delta), nil
	}
}
