package memqueue

import (
	"sync"
	"unsafe"
)

type MQ struct {
	out chan unsafe.Pointer

	//ring buffer
	mincap    int
	length    int
	buffer    []unsafe.Pointer
	headindex int
	tailindex int
	num       int

	closestatus bool
	sync.Mutex
}

func New(mincap int) *MQ {
	instance := new(MQ)
	if mincap < 64 {
		instance.mincap = 64
	} else {
		instance.mincap = mincap
	}
	instance.length = mincap
	instance.out = make(chan unsafe.Pointer, 1)
	instance.buffer = make([]unsafe.Pointer, mincap)
	instance.headindex = 0
	instance.tailindex = 0
	return instance
}

func (this *MQ) Put(data unsafe.Pointer) int {
	this.Lock()
	if this.closestatus {
		this.Unlock()
		return -1
	}
	if this.headindex == this.tailindex {
		//no buffer
		select {
		case this.out <- data:
		default:
			this.buffer[this.tailindex] = data
			this.tailindex++
			if this.tailindex >= this.length {
				this.tailindex = 0
			}
			this.num++
		}
	} else {
		//has buffer
		this.buffer[this.tailindex] = data
		this.tailindex++
		if this.tailindex >= this.length {
			this.tailindex = 0
		}
		//grow buffer
		if this.tailindex == this.headindex {
			tempbuffer := make([]unsafe.Pointer, this.length+this.mincap)
			for i := 0; i < this.length; i++ {
				tempbuffer[0] = this.buffer[this.headindex]
				this.headindex++
				if this.headindex >= this.length {
					this.headindex = 0
				}
			}
			this.buffer = tempbuffer
			this.headindex = 0
			this.tailindex = this.length
			this.length += this.mincap
		}
		this.num++
	}
	curnum := len(this.out) + this.num
	this.Unlock()
	return curnum
}

//return nil and 0 means closed
func (this *MQ) Get(notice chan int) (unsafe.Pointer, int) {
	if len(notice) > 0 {
		return nil, <-notice
	}
	select {
	case v := <-notice:
		return nil, v
	case v, ok := <-this.out:
		if !ok {
			return nil, 0
		}
		curnum := this.num
		this.Lock()
		if len(this.buffer) != 0 {
			this.out <- this.buffer[this.headindex]
			this.headindex++
			if this.headindex >= this.length {
				this.headindex = 0
			}
			this.num--
			if this.closestatus && this.num == 0 {
				close(this.out)
			}
		} else if this.closestatus {
			close(this.out)
		}
		this.Unlock()
		return v, curnum
	}
}
func (this *MQ) Close() {
	this.Lock()
	this.closestatus = true
	this.Unlock()
}
