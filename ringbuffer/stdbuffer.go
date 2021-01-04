package ringbuffer

import (
	"errors"
	"unsafe"
)

var (
	ERRFULL = errors.New("buffer is full")
)

//thread not safe,but can auto grow and shirnk within minlen and maxlen
type StdRingBuffer struct {
	head      int
	tail      int
	data      []unsafe.Pointer
	maxlen    int
	curlen    int
	minbuflen int
	maxbuflen int

	shirnkcount int
}

// if maxbuflen is 0,means no limit
func NewStdRingBuffer(minbuflen, maxbuflen int) *StdRingBuffer {
	if minbuflen <= 0 {
		minbuflen = 1024
	}
	if maxbuflen < minbuflen && maxbuflen != 0 {
		maxbuflen = minbuflen * 2
	}
	return &StdRingBuffer{
		head:      0,
		tail:      0,
		data:      make([]unsafe.Pointer, minbuflen),
		maxlen:    minbuflen,
		curlen:    0,
		minbuflen: minbuflen,
		maxbuflen: maxbuflen,
	}
}
func (b *StdRingBuffer) Pop(num int) []unsafe.Pointer {
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
func (b *StdRingBuffer) Peek(offset, num int) []unsafe.Pointer {
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
func (b *StdRingBuffer) Push(data []unsafe.Pointer) error {
	if r := b.Rest(); r != -1 && len(data) > r {
		return ERRFULL
	}
	//grow
	for b.maxlen-b.curlen < len(data) {
		var tempdata []unsafe.Pointer
		if b.maxbuflen != 0 && b.maxlen*2 >= b.maxbuflen {
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
func (b *StdRingBuffer) Num() int {
	return b.curlen
}

//return -1 means no limit
func (b *StdRingBuffer) Rest() int {
	if b.maxbuflen == 0 {
		return -1
	}
	return b.maxbuflen - b.curlen
}

func (b *StdRingBuffer) Reset() {
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
