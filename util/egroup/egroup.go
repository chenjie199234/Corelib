package egroup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var p sync.Pool

type Group struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	erronce atomic.Bool
	err     error
	newtask atomic.Bool
}

func GetGroup(ctx context.Context) *Group {
	ectx, ecancel := context.WithCancel(ctx)
	g, ok := p.Get().(*Group)
	if !ok {
		g = &Group{}
	}
	g.newtask.Store(true)
	g.ctx = ectx
	g.cancel = ecancel
	return g
}

// PutGroup will wait all goroutine exit
func PutGroup(g *Group) (e error) {
	g.newtask.Store(false)
	g.wg.Wait()
	g.cancel()
	e = g.err
	g.ctx = nil
	g.cancel = nil
	g.err = nil
	g.erronce.Store(false)
	p.Put(g)
	return
}

var ErrStopped = errors.New("[egroup] was not created by GetGroup or PutGroup already be called,can't start new task")

func (g *Group) Go(f func(context.Context) error) error {
	if f == nil {
		panic("[egroup] f == nil")
	}
	g.wg.Add(1)
	if !g.newtask.Load() {
		g.wg.Done()
		return ErrStopped
	}
	go func() {
		if e := f(g.ctx); e != nil && g.erronce.CompareAndSwap(false, true) {
			g.err = e
			g.cancel()
		}
		g.wg.Done()
	}()
	return nil
}
