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
	notice      chan struct{}
	timeout     time.Duration
	timeoutfunc func(t T)
	new         func(context.Context) (T, error)
}
type node[T any] struct {
	t    T
	tmer *time.Timer
}

// if max == 0,means no limit
// if timeout <= 0 means no timeout
// timeoutfunc used to tell user which element expired
func NewPool[T any](max uint32, new func(context.Context) (T, error), timeout time.Duration, timeoutfunc func(t T)) *Pool[T] {
	if new == nil {
		panic("missing new func,can't create element in the pool")
	}
	return &Pool[T]{
		p: &sync.Pool{
			New: func() any {
				return &node[T]{}
			},
		},
		l:           list.NewList[*node[T]](),
		max:         max,
		count:       0,
		notice:      make(chan struct{}, 1),
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
				r := n.t //copy first,then reuse the node struct
				var empty T
				n.t = empty
				n.tmer = nil
				p.p.Put(n)
				if p.max > 0 {
					//p.l.Put 10 times,only wake up one Get(because the notice's buf len is 1)
					//so we need to try to tell other Get to try again
					select {
					case p.notice <- struct{}{}:
					default:
					}
				}
				return r, nil
			}
			//has timeout
			if !n.tmer.Stop() {
				//already expired
				atomic.AddUint32(&p.count, math.MaxUint32)
				var empty T
				n.t = empty
				n.tmer = nil
				p.p.Put(n)
				continue
			}
			//not expired
			r := n.t //copy first,then reuse the node struct
			var empty T
			n.t = empty
			n.tmer = nil
			p.p.Put(n)
			if p.max > 0 {
				//p.l.Put 10 times,only wake up one Get(because the notice's buf len is 1)
				//so we need to try to tell other Get to try again
				select {
				case p.notice <- struct{}{}:
				default:
				}
			}
			return r, nil
		}
		//pool is empty now
		if p.max == 0 {
			//no limit
			tmp, e := p.new(ctx)
			if e == nil {
				atomic.AddUint32(&p.count, 1)
			}
			return tmp, e
		}
		//limit check
		for {
			oldcount := atomic.LoadUint32(&p.count)
			if oldcount >= p.max {
				//full
				break
			}
			if ctx.Err() != nil {
				var empty T
				return empty, ctx.Err()
			}
			if !atomic.CompareAndSwapUint32(&p.count, oldcount, oldcount+1) {
				continue
			}
			//not full,create new element
			tmp, e := p.new(ctx)
			if e != nil {
				//create failed,delete the count
				atomic.AddUint32(&p.count, math.MaxUint32)
				select {
				case p.notice <- struct{}{}:
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
	if abandon {
		atomic.AddUint32(&p.count, math.MaxUint32)
		if p.max > 0 {
			select {
			case p.notice <- struct{}{}:
			default:
			}
		}
		return
	}
	n := p.p.Get().(*node[T])
	n.t = t
	if p.timeout > 0 {
		n.tmer = time.AfterFunc(p.timeout, func() {
			if p.timeoutfunc != nil {
				p.timeoutfunc(t)
			}
		})
	}
	p.l.Push(n)
	if p.max > 0 {
		select {
		case p.notice <- struct{}{}:
		default:
		}
	}
}
