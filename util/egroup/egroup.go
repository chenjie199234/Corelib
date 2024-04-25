package egroup

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var p sync.Pool

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	err    error
	done   int32
	init   int32
}

func GetGroup(ctx context.Context) *Group {
	ectx, ecancel := context.WithCancel(ctx)
	g, ok := p.Get().(*Group)
	if ok {
		g.ctx = ectx
		g.cancel = ecancel
		g.done = 0
		g.err = nil
		g.init = 1
	} else {
		g = &Group{
			ctx:    ectx,
			cancel: ecancel,
			init:   1,
		}
	}
	return g
}

// PutGroup will wait all goroutine exit
func PutGroup(g *Group) error {
	defer p.Put(g)
	g.wg.Wait()
	g.cancel()
	g.init = 0
	return g.err
}

func (g *Group) Go(f func(context.Context) error) error {
	if g.init != 1 {
		return errors.New("[egroup] group must be created by GetGroup and didn't call PutGroup before use it")
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		e := f(g.ctx)
		if e != nil {
			if atomic.SwapInt32(&g.done, 1) == 0 {
				g.err = e
				if g.cancel != nil {
					g.cancel()
				}
			}
		}
	}()
	return nil
}
