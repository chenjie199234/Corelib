package logger

import (
	"fmt"
	"sync"
	"unsafe"
)

type MQ struct {
	name string

	out chan unsafe.Pointer

	//ring buffer
	mincap    int
	maxcap    int
	length    int
	buffer    []unsafe.Pointer
	headindex int
	tailindex int
	num       int32

	closestatus bool
	sync.Mutex
}

//maxcap is better to set as n * mincap
//if buffer is overflow,the msg will be drop
func NewMQ(name string, mincap, maxcap int) *MQ {
	instance := new(MQ)
	if mincap < 64 {
		instance.mincap = 64
	} else {
		instance.mincap = mincap
	}
	if maxcap <= instance.mincap {
		instance.maxcap = instance.mincap * 256
	} else {
		instance.maxcap = maxcap
	}
	instance.out = make(chan unsafe.Pointer, 1)
	instance.buffer = make([]unsafe.Pointer, instance.mincap)
	instance.length = instance.mincap
	instance.headindex = 0
	instance.tailindex = 0
	return instance
}

//return 0 means closed
func (this *MQ) Put(data unsafe.Pointer) int {
	this.Lock()
	if this.closestatus {
		this.Unlock()
		return 0
	}
	if this.num == 0 {
		//no buffer
		this.out <- data
	} else {
		//check free buffer
		if int(this.num) == this.length+1 {
			//on enough free buffer,this message will be drop
			curnum := int(this.num)
			this.Unlock()
			fmt.Printf("message queue:%s is full,new msg dropped!", this.name)
			return curnum
		}
		//has free buffer
		this.buffer[this.tailindex] = data
		this.tailindex++
		if this.tailindex >= this.length {
			this.tailindex = 0
		}
		//grow buffer
		if this.tailindex == this.headindex {
			if this.length != this.maxcap {
				if this.length+this.mincap > this.maxcap {
					tempbuffer := make([]unsafe.Pointer, this.maxcap)
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
					this.length = this.maxcap
				} else {
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
			} else {
				//already grow to the max,do nothing
			}
		}
	}
	this.num++
	curnum := int(this.num)
	this.Unlock()
	return curnum
}

//the data in notice chan should < 0
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
		return v, int(this.num)
	}
}
func (this *MQ) Close() {
	this.Lock()
	this.closestatus = true
	this.Unlock()
}
func (this *MQ) Num() int {
	return int(this.num)
}
