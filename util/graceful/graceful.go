package graceful

import (
	"errors"
	"math"
	"sync/atomic"
)

type Graceful struct {
	progress int64
	stop     chan *struct{}
}

func New() *Graceful {
	return &Graceful{
		progress: 0,
		stop:     make(chan *struct{}),
	}
}

var ErrClosing = errors.New("[Graceful.Add] closing")
var ErrMax = errors.New("[Graceful.Add] out of bounds")

func (g *Graceful) Add(delta uint16) error {
	if delta == 0 {
		panic("[Graceful.Add] delta is 0")
	}
	for {
		old := atomic.LoadInt64(&g.progress)
		if old < 0 {
			return ErrClosing
		}
		if old+int64(delta) < 0 {
			return ErrMax
		}
		if atomic.CompareAndSwapInt64(&g.progress, old, old+int64(delta)) {
			return nil
		}
	}
}
func (g *Graceful) DoneOne() {
	if atomic.AddInt64(&g.progress, -1) == math.MinInt64 {
		close(g.stop)
	}
}
func (g *Graceful) Close(cleanOnceNow func(), cleanOnceAfter func()) {
	first := false
	for {
		old := atomic.LoadInt64(&g.progress)
		if old < 0 {
			break
		}
		if first = atomic.CompareAndSwapInt64(&g.progress, old, old+math.MinInt64); first {
			if cleanOnceNow != nil {
				cleanOnceNow()
			}
			if old == 0 {
				close(g.stop)
			}
			break
		}
	}
	<-g.stop
	if first && cleanOnceAfter != nil {
		cleanOnceAfter()
	}
}
func (g *Graceful) Closing() bool {
	return g.progress < 0
}
func (g *Graceful) Closed() bool {
	return g.progress == math.MinInt64
}
func (g *Graceful) GetNum() int64 {
	progress := atomic.LoadInt64(&g.progress)
	if progress < 0 {
		return progress - math.MinInt64
	}
	return progress
}
