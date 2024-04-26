package bpool

import (
	"sync"
)

var p *sync.Pool

func init() {
	p = &sync.Pool{}
}

// get a []byte which len() == 0 and cap() >= length
func Get(length int) []byte {
	if length < 0 {
		panic("cannot be  negative")
	} else if length < 256 {
		length = 256
	}
	if b, ok := p.Get().([]byte); ok {
		if cap(b) >= length {
			return b[:0]
		}
		p.Put(&b)
	}
	return make([]byte, 0, length)
}
func Put(b *[]byte) {
	*b = (*b)[:0]
	p.Put(*b)
}

// if b's cap < length,a new []byte will return with b's data be copyed
// if b's cap >= length,return b
func CheckCap(b *[]byte, length int) []byte {
	if length > cap(*b) {
		//overflow! need a new buf
		tmp := Get(length)
		tmp = tmp[:len(*b)]
		copy(tmp, *b)
		Put(b)
		return tmp
	}
	return *b
}

// ------------------------------------------------------------------------------

type pool struct{}

func (p *pool) Get(lenght int) []byte {
	return Get(lenght)
}
func (p *pool) Put(b *[]byte) {
	Put(b)
}
func GetPool() *pool {
	return &pool{}
}
