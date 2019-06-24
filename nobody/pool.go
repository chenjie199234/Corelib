package nobody

import (
	"compress/gzip"
	"io"
	"sync"
)

type pool struct {
	gzipwPool   *sync.Pool
	contextPool *sync.Pool
}

var poolInstance *pool

func init() {
	poolInstance = new(pool)
	poolInstance.gzipwPool = &sync.Pool{
		New: func() interface{} { temp, _ := gzip.NewWriterLevel(nil, gzip.BestCompression); return temp },
	}
	poolInstance.contextPool = &sync.Pool{
		New: func() interface{} { return new(Context) },
	}
}
func (p *pool) getContext() *Context {
	return p.contextPool.Get().(*Context)
}
func (p *pool) putContext(c *Context) {
	c.reset()
	p.contextPool.Put(c)
}
func (p *pool) getGzipWriter(w io.Writer) *gzip.Writer {
	temp := p.gzipwPool.Get().(*gzip.Writer)
	temp.Reset(w)
	return temp
}
func (p *pool) putGzipWriter(w *gzip.Writer) {
	w.Reset(nil)
	p.gzipwPool.Put(w)
}
