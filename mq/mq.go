//memory message queue
//Get,Put,Close function doesn't have data race
//Num,Rest have data race
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

	minbuflen int
	maxbuflen int
	maxlen    int
	curlen    int
	data      []unsafe.Pointer
	head      int
	tail      int

	shirnkcount int

	closestatus bool
	sync.Mutex
}

//mq is closed by default after be created,please reset it
func NewMQPool(minbuflen, maxbuflen int) *sync.Pool {
	if minbuflen <= 0 {
		minbuflen = 64
	}
	if maxbuflen < minbuflen {
		maxbuflen = minbuflen * 2
	}
	return &sync.Pool{
		New: func() interface{} {
			mq := &MQ{
				out: make(chan unsafe.Pointer, 1),

				minbuflen: minbuflen,
				maxbuflen: maxbuflen,
				maxlen:    minbuflen,
				curlen:    0,
				data:      make([]unsafe.Pointer, minbuflen),
				head:      0,
				tail:      0,

				closestatus: true,
			}
			close(mq.out)
			return mq
		},
	}
}

//mq is closed by default after be created,please reset it
func NewMQ(minbuflen, maxbuflen int) *MQ {
	if minbuflen <= 0 {
		minbuflen = 64
	}
	if maxbuflen < minbuflen {
		maxbuflen = minbuflen * 2
	}
	mq := &MQ{
		out: make(chan unsafe.Pointer, 1),

		minbuflen: minbuflen,
		maxbuflen: maxbuflen,
		maxlen:    minbuflen,
		curlen:    0,
		data:      make([]unsafe.Pointer, minbuflen),
		head:      0,
		tail:      0,

		closestatus: true,
	}
	close(mq.out)
	return mq
}

//free means you can put how many elements into this mq continue
func (this *MQ) Put(data unsafe.Pointer) (free int, e error) {
	this.Lock()
	if this.closestatus {
		e = ERRCLOSED
		this.Unlock()
		return
	}
	if this.Rest() == 0 {
		e = ERRFULL
		this.Unlock()
		return
	}
	if this.curlen == 0 {
		select {
		case this.out <- data:
			//shirnk
			this.shirnkcount++
			this.shirnk()
			free = this.maxbuflen - this.curlen
			this.Unlock()
			return
		default:
		}
	}
	//grow
	this.grow()
	//input
	this.data[this.tail] = data
	this.curlen++
	this.tail++
	if this.tail >= this.maxlen {
		this.tail = 0
	}
	//shirnk
	if float64(this.curlen) < float64(this.maxlen)/3.0 {
		this.shirnkcount++
	} else {
		this.shirnkcount = 0
	}
	this.shirnk()
	this.Unlock()
	free = this.maxbuflen - this.curlen
	return
}
func (this *MQ) grow() {
	if this.curlen == this.maxlen {
		var tempdata []unsafe.Pointer
		if this.maxlen*2 >= this.maxbuflen {
			tempdata = make([]unsafe.Pointer, this.maxbuflen)
		} else {
			tempdata = make([]unsafe.Pointer, this.maxlen*2)
		}
		for i := 0; i < this.curlen; i++ {
			if this.head+i >= this.maxlen {
				tempdata[i] = this.data[this.head+i-this.maxlen]
			} else {
				tempdata[i] = this.data[this.head+i]
			}
		}
		this.data = tempdata
		this.head = 0
		this.tail = this.curlen
		this.maxlen = len(tempdata)
	}
}
func (this *MQ) shirnk() {
	if this.shirnkcount >= 50 && this.maxlen > this.minbuflen {
		this.shirnkcount = 0
		var tempdata []unsafe.Pointer
		if this.maxlen/2 <= this.minbuflen {
			tempdata = make([]unsafe.Pointer, this.minbuflen)
		} else {
			tempdata = make([]unsafe.Pointer, this.maxlen/2)
		}
		for i := 0; i < this.curlen; i++ {
			if this.head+i >= this.maxlen {
				tempdata[i] = this.data[this.head+i-this.maxlen]
			} else {
				tempdata[i] = this.data[this.head+i]
			}
		}
		this.data = tempdata
		this.head = 0
		this.tail = this.curlen
		this.maxlen = len(tempdata)
	}
}

//rest means you can get how many elements from this mq continue
func (this *MQ) Get() (data unsafe.Pointer, rest int) {
	select {
	case v, ok := <-this.out:
		if !ok {
			return nil, -1
		}
		rest = this.curlen
		data = v
		this.Lock()
		if this.curlen > 0 {
			this.out <- this.data[this.head]
			this.curlen--
			this.head++
			if this.head >= this.maxlen {
				this.head = 0
			}
		} else if this.closestatus {
			close(this.out)
		}
		this.Unlock()
		return
	}
}
func (this *MQ) Close() {
	this.Lock()
	this.closestatus = true
	if this.curlen == 0 {
		close(this.out)
	}
	this.Unlock()
}

//have data race
func (this *MQ) Num() int {
	return this.curlen + len(this.out)
}

//have data race
func (this *MQ) Rest() int {
	return this.maxbuflen + 1 - this.curlen - len(this.out)
}

//this shouldn't be used before put back into the pool
//this should be used after new mq or get from pool
func (this *MQ) Reset() {
	this.out = make(chan unsafe.Pointer, 1)

	this.maxlen = this.minbuflen
	this.curlen = 0
	this.data = this.data[:this.minbuflen]
	this.head = 0
	this.tail = 0

	this.closestatus = false
}
