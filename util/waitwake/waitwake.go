package waitwake

import (
	"context"
	"sync"
)

type WaitWake struct {
	lker    *sync.Mutex
	notices map[string]*notice
}
type notice struct {
	ch    chan *struct{}
	count uint64
}

func NewWaitWake() *WaitWake {
	return &WaitWake{
		lker:    &sync.Mutex{},
		notices: make(map[string]*notice),
	}
}

// doOnce and doEvery will run in different goroutine,so there has no order
// the doDoce function(if doOnce is not nil) will run only when there is no Wait on the key now
// the doEvery function(if doEvery is not nil) will run every time call the Wait
func (w *WaitWake) Wait(ctx context.Context, key string, doOnce func(), doEvery func()) error {
	w.lker.Lock()
	n, ok := w.notices[key]
	if !ok {
		n = &notice{
			ch:    make(chan *struct{}),
			count: 1,
		}
		w.notices[key] = n
		if doOnce != nil {
			go doOnce()
		}
	} else {
		n.count++
	}
	if doEvery != nil {
		go doEvery()
	}
	w.lker.Unlock()
	select {
	case <-ctx.Done():
		w.lker.Lock()
		n.count--
		if n.count == 0 {
			delete(w.notices, key)
		}
		w.lker.Unlock()
		return ctx.Err()
	case <-n.ch:
		return nil
	}
}
func (w *WaitWake) Wake(key string) {
	w.lker.Lock()
	defer w.lker.Unlock()
	if n, ok := w.notices[key]; ok {
		close(n.ch)
		delete(w.notices, key)
	}
}
