package cpool

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/container/list"
)

type CPool[T any] struct {
	p           *sync.Pool
	l           *list.List[*node[T]]
	max         uint32
	count       uint32
	notice      chan *struct{}
	timeout     time.Duration
	timeoutfunc func(t T)
	new         func() (T, error)
}
type node[T any] struct {
	t    T
	tmer *time.Timer
}

// if max == 0,means no limit
// if timeout <= 0 or timeoutfunc == nil,means no timeout
func NewCPool[T any](max uint32, new func() (T, error), timeout time.Duration, timeoutfunc func(t T)) *CPool[T] {
	if new == nil {
		panic("missing new func,can't create element in the pool")
	}
	return &CPool[T]{
		p:           &sync.Pool{},
		l:           list.NewList[*node[T]](),
		max:         max,
		count:       0,
		notice:      make(chan *struct{}, 1),
		timeout:     timeout,
		timeoutfunc: timeoutfunc,
		new:         new,
	}
}
func (p *CPool[T]) Get(ctx context.Context) (T, error) {
	for {
		n, e := p.l.Pop(nil)
		if e == nil {
			if p.timeout <= 0 {
				//no timeout
				select {
				case p.notice <- nil:
				default:
				}
				p.p.Put(n)
				return n.t, nil
			}
			//has timeout
			if !n.tmer.Stop() {
				atomic.AddUint32(&p.count, math.MaxUint32)
				p.p.Put(n)
				continue
			}
			select {
			case p.notice <- nil:
			default:
			}
			p.p.Put(n)
			return n.t, nil
		}
		if p.max == 0 {
			//no limit
			atomic.AddUint32(&p.count, 1)
			return p.new()
		}
		//limit check
		for {
			oldcount := p.count
			if oldcount >= p.max {
				break
			}
			if !atomic.CompareAndSwapUint32(&p.count, oldcount, oldcount+1) {
				continue
			}
			tmp, e := p.new()
			if e != nil {
				atomic.AddUint32(&p.count, math.MaxUint32)
				select {
				case p.notice <- nil:
				default:
				}
			} else if atomic.LoadUint32(&p.count) < p.max {
				select {
				case p.notice <- nil:
				default:
				}
			}
			return tmp, e
		}
		select {
		case <-ctx.Done():
			var t T
			return t, ctx.Err()
		case <-p.notice:
		}
	}
}
func (p *CPool[T]) Put(t T) {
	n, ok := p.p.Get().(*node[T])
	if !ok {
		n = &node[T]{}
	}
	n.t = t
	if p.timeout > 0 && p.timeoutfunc != nil {
		n.tmer = time.AfterFunc(p.timeout, func() {
			n.tmer.Stop()
			p.timeoutfunc(t)
		})
	}
	p.l.Push(n)
	select {
	case p.notice <- nil:
	default:
	}
}
func (p *CPool[T]) AbandonOne() {
	atomic.AddUint32(&p.count, math.MaxUint32)
	select {
	case p.notice <- nil:
	default:
	}
}
