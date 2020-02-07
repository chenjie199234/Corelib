package mq

import (
	"errors"
	"sync"
	"unsafe"
)

var (
	ERRFULL   = errors.New("mq is full,new msg will be dropped")
	ERRCLOSED = errors.New("mq is closed")
)

type MQ struct {
	out chan unsafe.Pointer

	//ring buffer
	mincap    int
	maxcap    int
	length    int
	buffer    []unsafe.Pointer
	headindex int
	tailindex int
	num       int

	closestatus bool
	sync.Mutex
}

//maxcap is better to set as n * mincap
//if buffer is overflow,the msg will be drop
func New(mincap, maxcap int) *MQ {
	instance := new(MQ)
	if mincap < 64 {
		instance.mincap = 64
	} else {
		instance.mincap = mincap
	}
	if maxcap <= instance.mincap*2 {
		instance.maxcap = instance.mincap * 2
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

func (this *MQ) Put(data unsafe.Pointer) (left int, e error) {
	this.Lock()
	if this.closestatus {
		e = ERRCLOSED
		left = this.num
		this.Unlock()
		return
	}
	if this.num == 0 {
		//no buffer
		this.out <- data
	} else {
		//check free buffer(+1 for the space in chan)
		if int(this.num) == this.length+1 {
			//no enough free buffer,this message will be drop
			e = ERRFULL
			left = this.num
			this.Unlock()
			return
		}
		//has free buffer
		this.buffer[this.tailindex] = data
		this.tailindex++
		if this.tailindex >= this.length {
			this.tailindex = 0
		}
		//grow buffer
		if this.tailindex == this.headindex && this.length != this.maxcap {
			targetlength := 0
			if this.length+this.mincap > this.maxcap {
				targetlength = this.maxcap
			} else {
				targetlength = this.length + this.mincap
			}
			tempbuffer := make([]unsafe.Pointer, targetlength)
			for i := 0; i < this.length; i++ {
				tempbuffer[i] = this.buffer[this.headindex]
				this.headindex++
				if this.headindex >= this.length {
					this.headindex = 0
				}
			}
			this.buffer = tempbuffer
			this.headindex = 0
			this.tailindex = this.length
			this.length = targetlength
		}
	}
	this.num++
	left = this.num
	this.Unlock()
	return
}

//'data' == nil make 'leftornotice' to notice,value of 'leftornotice' decide the meaning of the notice,'-1' means this mq is closed
//'data' != nil make 'leftornotice' to left,value of 'leftornotice' means the left message num in this mq
func (this *MQ) Get(notice chan uint) (data unsafe.Pointer, leftornotice int) {
	if len(notice) > 0 {
		return nil, int(<-notice)
	}
	select {
	case v := <-notice:
		return nil, int(v)
	case v, ok := <-this.out:
		if !ok {
			return nil, -1
		}
		this.Lock()
		this.num--
		if this.num > 0 {
			this.out <- this.buffer[this.headindex]
			this.headindex++
			if this.headindex >= this.length {
				this.headindex = 0
			}
		} else if this.closestatus {
			close(this.out)
		}
		//shirnk buffer
		if !this.closestatus && this.num <= (this.length/3) && this.length > this.mincap {
			targetlength := 0
			if this.length-this.mincap < this.mincap {
				targetlength = this.mincap
			} else {
				targetlength = this.length - this.mincap
			}
			tempbuffer := make([]unsafe.Pointer, targetlength)
			for i := 0; i < this.num; i++ {
				tempbuffer[i] = this.buffer[this.headindex]
				this.headindex++
				if this.headindex >= this.length {
					this.headindex = 0
				}
			}
			this.buffer = tempbuffer
			this.headindex = 0
			this.tailindex = this.num
			this.length = targetlength
		}
		data = v
		leftornotice = this.num
		this.Unlock()
		return
	}
}
func (this *MQ) Close() {
	this.Lock()
	this.closestatus = true
	if this.num == 0 {
		close(this.out)
	}
	this.Unlock()
}
func (this *MQ) Num() int {
	return this.num
}
