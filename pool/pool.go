package pool

import (
	"bufio"
	"io"
	"sync"
)

type innerPool struct {
	buf     *sync.Pool
	breader *sync.Pool
	bwriter *sync.Pool
}

var inner *innerPool

func init() {
	inner = &innerPool{
		buf:     &sync.Pool{},
		breader: &sync.Pool{},
		bwriter: &sync.Pool{},
	}
}
func GetPool() *innerPool {
	return inner
}

var begin = uint64(256)
var m128 = uint64(1024 * 1024 * 128)
var m972 = uint64(float64(m128) * 1.5 * 1.5 * 1.5 * 1.5 * 1.5)

func next(reqsize, nowcap uint64) uint64 {
	for nowcap < reqsize {
		if nowcap >= m972 {
			nowcap = uint64(float64(nowcap) * 1.25)
		} else if nowcap >= m128 {
			nowcap = uint64(float64(nowcap) * 1.5)
		} else {
			nowcap *= 2
		}
	}
	return nowcap
}

// this will return a []byte who's len() == length,but the cap() >= length
func (p *innerPool) Get(length int) []byte {
	b, ok := p.buf.Get().([]byte)
	if ok {
		if cap(b) > length {
			b = b[:length]
			return b
		}
		p.buf.Put(&b)
	}
	return make([]byte, length, next(uint64(length), begin))
}
func (p *innerPool) Put(b *[]byte) {
	*b = (*b)[:0]
	p.buf.Put(*b)
}
func CheckCap(b *[]byte, length int) []byte {
	if length > cap(*b) {
		//overflow! need a new buf
		tmp := inner.Get(length)
		tmp = tmp[:len(*b)]
		copy(tmp, *b)
		inner.Put(b)
		return tmp
	}
	return *b
}
func (p *innerPool) GetBufReader(reader io.Reader) *bufio.Reader {
	tmp, ok := p.breader.Get().(*bufio.Reader)
	if !ok {
		return bufio.NewReader(reader)
	}
	tmp.Reset(reader)
	return tmp
}
func (p *innerPool) PutBufReader(breader *bufio.Reader) {
	breader.Reset(nil)
	p.breader.Put(breader)
}
func (p *innerPool) GetBufWriter(writer io.Writer) *bufio.Writer {
	tmp, ok := p.bwriter.Get().(*bufio.Writer)
	if !ok {
		return bufio.NewWriter(writer)
	}
	tmp.Reset(writer)
	return tmp
}
func (p *innerPool) PutBufWriter(bwriter *bufio.Writer) {
	bwriter.Reset(nil)
	p.bwriter.Put(bwriter)
}
