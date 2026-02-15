package pool

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/container/list"
)

type Pool[T any] struct {
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
func NewCPool[T any](max uint32, new func() (T, error), timeout time.Duration, timeoutfunc func(t T)) *Pool[T] {
	if new == nil {
		panic("missing new func,can't create element in the pool")
	}
	return &Pool[T]{
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

// the returned T must return back to this pool by call Put() after it's business finished.
// if not,the max number check will broken
func (p *Pool[T]) Get(ctx context.Context) (T, error) {
	for {
		n, e := p.l.Pop(nil)
		if e == nil {
			if p.timeout <= 0 {
				//no timeout
				select {
				case p.notice <- nil:
				default:
				}
				r := n.t //copy first,then reuse the node struct
				var empty T
				n.t = empty
				p.p.Put(n)
				return r, nil
			}
			//has timeout
			if !n.tmer.Stop() {
				//already expired
				atomic.AddUint32(&p.count, math.MaxUint32)
				var empty T
				n.t = empty
				p.p.Put(n)
				continue
			}
			//not expired
			select {
			case p.notice <- nil:
			default:
			}
			r := n.t //copy first,then reuse the node struct
			var empty T
			n.t = empty
			p.p.Put(n)
			return r, nil
		}
		//pool is empty now
		if p.max == 0 {
			//no limit
			tmp, e := p.new()
			if e == nil {
				atomic.AddUint32(&p.count, 1)
			}
			return tmp, e
		}
		//limit check
		for {
			oldcount := p.count
			if oldcount >= p.max {
				//full
				break
			}
			if !atomic.CompareAndSwapUint32(&p.count, oldcount, oldcount+1) {
				continue
			}
			if atomic.LoadUint32(&p.count) < p.max {
				select {
				case p.notice <- nil:
				default:
				}
			}
			//not full,create new element
			tmp, e := p.new()
			if e != nil {
				//create failed,delete the count
				atomic.AddUint32(&p.count, math.MaxUint32)
				select {
				case p.notice <- nil:
				default:
				}
			}
			return tmp, e
		}
		select {
		case <-ctx.Done():
			var empty T
			return empty, ctx.Err()
		case <-p.notice:
		}
	}
}

// this function should be called when the Get() returned T finished it's business.
// abandon:this T doesn't need anymore.
// example:this is a connection pool,if the connection from Get() is broken or closed,the abandon should be true.
func (p *Pool[T]) Put(t T, abandon bool) {
	if abandon && p.max > 0 {
		atomic.AddUint32(&p.count, math.MaxUint32)
		select {
		case p.notice <- nil:
		default:
		}
		return
	}
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
