package lru

import (
	"container/list"
	"sync"
	"time"
)

// thread unsafe
type LruCache[T any] struct {
	mcap uint32
	ccap uint32
	ttl  time.Duration
	bufm map[any]*list.Element
	bufl *list.List
	pool *sync.Pool
}
type ttlValue[T any] struct {
	ttl   int64 //unixnano
	key   any
	value T
}

// ttl == 0 means not using expire time
func New[T any](maxcap uint32, ttl time.Duration) *LruCache[T] {
	if maxcap == 0 || ttl < 0 {
		return nil
	}
	return &LruCache[T]{
		mcap: maxcap,
		ccap: 0,
		ttl:  ttl,
		bufm: make(map[any]*list.Element, maxcap),
		bufl: list.New(),
		pool: &sync.Pool{
			New: func() any { return &ttlValue[T]{} },
		},
	}
}

func (l *LruCache[T]) Get(key any) (data T, ok bool) {
	elm, ok := l.bufm[key]
	if !ok {
		return
	}
	vv := elm.Value.(*ttlValue[T])
	if l.ttl != 0 {
		now := time.Now()
		if vv.ttl <= now.UnixNano() {
			//timeout
			l.bufl.Remove(elm)
			delete(l.bufm, key)
			l.ccap--
			l.putPool(vv)
			return
		}
		vv.ttl = now.Add(l.ttl).UnixNano()
		l.bufl.MoveToFront(elm)
		//lazy clean,every time remove one element
		if back := l.bufl.Back(); back != nil && back != elm {
			if vv := back.Value.(*ttlValue[T]); vv.ttl <= now.UnixNano() {
				l.bufl.Remove(back)
				delete(l.bufm, vv.key)
				l.ccap--
				l.putPool(vv)
			}
		}
	} else {
		l.bufl.MoveToFront(elm)
	}
	return vv.value, true
}
func (l *LruCache[T]) Set(key any, value T) {
	if elm, ok := l.bufm[key]; ok {
		vv := elm.Value.(*ttlValue[T])
		vv.value = value
		if l.ttl != 0 {
			now := time.Now()
			vv.ttl = now.Add(l.ttl).UnixNano()
			l.bufl.MoveToFront(elm)
			//lazy clean,every time remove one element
			if back := l.bufl.Back(); back != nil && back != elm {
				if vv := back.Value.(*ttlValue[T]); vv.ttl <= now.UnixNano() {
					l.bufl.Remove(back)
					delete(l.bufm, vv.key)
					l.ccap--
					l.putPool(vv)
				}
			}
		} else {
			l.bufl.MoveToFront(elm)
		}
	} else {
		if l.ttl != 0 {
			//lazy clean,every time remove one element
			if back := l.bufl.Back(); back != nil {
				if vv := back.Value.(*ttlValue[T]); vv.ttl <= time.Now().UnixNano() {
					l.bufl.Remove(back)
					delete(l.bufm, vv.key)
					l.ccap--
					l.putPool(vv)
				}
			}
		}
		if l.ccap >= l.mcap {
			if back := l.bufl.Back(); back != nil {
				vv := l.bufl.Remove(back).(*ttlValue[T])
				delete(l.bufm, vv.key)
				l.ccap--
				l.putPool(vv)
			}
		}
		vv := l.getPool()
		vv.key = key
		vv.value = value
		if l.ttl != 0 {
			vv.ttl = time.Now().Add(l.ttl).UnixNano()
		}
		l.bufm[key] = l.bufl.PushFront(vv)
		l.ccap++
	}
}
func (l *LruCache[T]) Del(key any) {
	elm, ok := l.bufm[key]
	if !ok {
		return
	}
	l.bufl.Remove(elm)
	delete(l.bufm, key)
	l.ccap--
	l.putPool(elm.Value.(*ttlValue[T]))
}
func (l *LruCache[T]) Len() uint32 {
	return l.ccap
}
func (l *LruCache[T]) Has(key any) bool {
	elm, ok := l.bufm[key]
	if l.ttl == 0 || !ok {
		return ok
	}
	return elm.Value.(*ttlValue[T]).ttl > time.Now().UnixNano()
}
func (l *LruCache[T]) getPool() *ttlValue[T] {
	return l.pool.Get().(*ttlValue[T])
}
func (l *LruCache[T]) putPool(v *ttlValue[T]) {
	v.ttl = 0
	var empty T
	v.value = empty
	v.key = nil
	l.pool.Put(v)
}
