//has data race

package ringbuffer

import (
	"errors"
	"sync"
	"unsafe"
)

var (
	ERRFULL = errors.New("buffer is full,new msg will be dropped")
)

type RingBuffer struct {
	head      int
	tail      int
	data      []unsafe.Pointer
	maxlen    int
	curlen    int
	minbuflen int
	maxbuflen int

	shirnkcount int
}

func NewBufPool(minbuflen, maxbuflen int) *sync.Pool {
	if minbuflen <= 0 {
		minbuflen = 1024
	}
	if maxbuflen <= minbuflen {
		maxbuflen = minbuflen * 2
	}
	return &sync.Pool{
		New: func() interface{} {
			return &RingBuffer{
				head:      0,
				tail:      0,
				data:      make([]unsafe.Pointer, minbuflen),
				maxlen:    minbuflen,
				curlen:    0,
				minbuflen: minbuflen,
				maxbuflen: maxbuflen,
			}
		},
	}
}
func NewBuf(minbuflen, maxbuflen int) *RingBuffer {
	if minbuflen <= 0 {
		minbuflen = 1024
	}
	if maxbuflen < minbuflen {
		maxbuflen = minbuflen * 2
	}
	return &RingBuffer{
		head:      0,
		tail:      0,
		data:      make([]unsafe.Pointer, minbuflen),
		maxlen:    minbuflen,
		curlen:    0,
		minbuflen: minbuflen,
		maxbuflen: maxbuflen,
	}
}
func (b *RingBuffer) Get(num int) []unsafe.Pointer {
	if num > b.curlen || num <= 0 {
		return nil
	}
	result := make([]unsafe.Pointer, num)
	for i := 0; i < num; i++ {
		result[i] = b.data[b.head]
		b.curlen--
		b.head++
		if b.head >= b.maxlen {
			b.head = 0
		}
	}
	return result
}
func (b *RingBuffer) Peek(offset, num int) []unsafe.Pointer {
	if num <= 0 || offset < 0 || offset+num > b.curlen {
		return nil
	}
	result := make([]unsafe.Pointer, num)
	for i := 0; i < num; i++ {
		if b.head+offset+i >= b.maxlen {
			result[i] = b.data[b.head+offset+i-b.maxlen]
		} else {
			result[i] = b.data[b.head+offset+i]
		}
	}
	return result
}
func (b *RingBuffer) Put(data []unsafe.Pointer) error {
	if len(data) > b.Rest() {
		return ERRFULL
	}
	//grow
	for b.maxlen-b.curlen < len(data) {
		var tempdata []unsafe.Pointer
		if b.maxlen*2 >= b.maxbuflen {
			tempdata = make([]unsafe.Pointer, b.maxbuflen)
		} else {
			tempdata = make([]unsafe.Pointer, b.maxlen*2)
		}
		for i := 0; i < b.curlen; i++ {
			if b.head+i >= b.maxlen {
				tempdata[i] = b.data[b.head+i-b.maxlen]
			} else {
				tempdata[i] = b.data[b.head+i]
			}
		}
		b.data = tempdata
		b.head = 0
		b.tail = b.curlen
		b.maxlen = len(tempdata)
	}
	//input
	for _, v := range data {
		b.data[b.tail] = v
		b.curlen++
		b.tail++
		if b.tail >= b.maxlen {
			b.tail = 0
		}
	}
	//shirnk
	if float64(b.curlen) < float64(b.maxlen)/3.0 {
		b.shirnkcount++
	} else {
		b.shirnkcount = 0
	}
	if b.shirnkcount >= 50 && b.maxlen > b.minbuflen {
		b.shirnkcount = 0
		var tempdata []unsafe.Pointer
		if b.maxlen/2 <= b.minbuflen {
			tempdata = make([]unsafe.Pointer, b.minbuflen)
		} else {
			tempdata = make([]unsafe.Pointer, b.maxlen/2)
		}
		for i := 0; i < b.curlen; i++ {
			if b.head+i >= b.maxlen {
				tempdata[i] = b.data[b.head+i-b.maxlen]
			} else {
				tempdata[i] = b.data[b.head+i]
			}
		}
		b.data = tempdata
		b.head = 0
		b.tail = b.curlen
		b.maxlen = len(tempdata)
	}
	return nil
}
func (b *RingBuffer) Num() int {
	return b.curlen
}
func (b *RingBuffer) Rest() int {
	return b.maxbuflen - b.curlen
}

//this should be used before put back into the pool,after new buffer and after get from pool
func (b *RingBuffer) Reset() {
	b.head = 0
	b.tail = 0
	if b.maxlen >= b.minbuflen*4 {
		b.data = make([]unsafe.Pointer, b.minbuflen) //free old mem
	} else {
		b.data = b.data[:b.minbuflen] //hold old mem as it's cap
	}
	b.maxlen = b.minbuflen
	b.curlen = 0
	b.shirnkcount = 0
}
