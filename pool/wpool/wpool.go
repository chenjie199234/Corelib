package wpool

import (
	"bufio"
	"io"
	"sync"
)

var p *sync.Pool

func init() {
	p = &sync.Pool{}
}

// get the bufio.Writer
func Get(writer io.Writer) *bufio.Writer {
	bw, ok := p.Get().(*bufio.Writer)
	if !ok {
		return bufio.NewWriter(writer)
	}
	bw.Reset(writer)
	return bw
}
func Put(b *bufio.Writer) {
	b.Flush()
	b.Reset(nil)
	p.Put(b)
}
