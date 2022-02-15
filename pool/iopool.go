package pool

import (
	"bufio"
	"io"
	"sync"
)

var readerpool *sync.Pool
var writerpool *sync.Pool

func init() {
	readerpool = &sync.Pool{}
	writerpool = &sync.Pool{}
}
func GetReader(r io.Reader) *bufio.Reader {
	tmp, ok := readerpool.Get().(*bufio.Reader)
	if !ok {
		return bufio.NewReader(r)
	}
	tmp.Reset(r)
	return tmp
}
func PutReader(r *bufio.Reader) {
	r.Reset(nil)
	readerpool.Put(r)
}
func GetWriter(w io.Writer) *bufio.Writer {
	tmp, ok := writerpool.Get().(*bufio.Writer)
	if !ok {
		return bufio.NewWriter(w)
	}
	tmp.Reset(w)
	return tmp
}
func PutWriter(w *bufio.Writer) {
	w.Flush()
	w.Reset(nil)
	writerpool.Put(w)
}
