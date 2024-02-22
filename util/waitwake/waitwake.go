package waitwake

import (
	"context"
	"sync"
)

type WaitWake struct {
	lker    *sync.Mutex
	notices map[string]map[chan *struct{}]*struct{}
}

func NewWaitWake() *WaitWake {
	return &WaitWake{
		lker:    &sync.Mutex{},
		notices: make(map[string]map[chan *struct{}]*struct{}, 10),
	}
}

// doOnce and doEvery will run in goroutine
// the doDoce function(if doOnce is not nil) will run only when there is no Wait on the key now
// the doEvery function(if doEvery is not nil) will run every time call the Wait
func (w *WaitWake) Wait(ctx context.Context, key string, doOnce func(), doEvery func()) error {
	notice := make(chan *struct{}, 1)
	w.lker.Lock()
	notices, ok := w.notices[key]
	if !ok {
		notices = make(map[chan *struct{}]*struct{}, 10)
		w.notices[key] = notices
	}
	notices[notice] = nil
	if len(notices) == 1 {
		if doOnce != nil {
			go doOnce()
		}
	}
	if doEvery != nil {
		go doEvery()
	}
	w.lker.Unlock()
	select {
	case <-ctx.Done():
		w.lker.Lock()
		delete(notices, notice)
		if len(notices) == 0 {
			delete(w.notices, key)
		}
		w.lker.Unlock()
		return ctx.Err()
	case <-notice:
		return nil
	}
}
func (w *WaitWake) Wake(key string) {
	w.lker.Lock()
	defer w.lker.Unlock()
	for notice := range w.notices[key] {
		delete(w.notices[key], notice)
		if notice != nil {
			notice <- nil
		}
	}
	delete(w.notices, key)
}
