package rpool

import (
	"bufio"
	"io"
	"sync"
)

var p *sync.Pool

func init() {
	p = &sync.Pool{}
}

// get the bufio.Reader
func Get(reader io.Reader) *bufio.Reader {
	br, ok := p.Get().(*bufio.Reader)
	if !ok {
		return bufio.NewReader(reader)
	}
	br.Reset(reader)
	return br
}
func Put(b *bufio.Reader) {
	b.Reset(nil)
	p.Put(b)
}
