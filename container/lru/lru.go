package lru

import (
	"sync"
	"time"
)

// thread unsafe
type LruCache[T any] struct {
	maxcap int64
	curcap int64
	ttl    time.Duration
	buf    map[any]*node[T]
	head   *node[T]
	tail   *node[T]
	pool   *sync.Pool
}
type kv[T any] struct {
	ttl   int64 //unixnano
	key   any
	value T
}

type node[T any] struct {
	data *kv[T]
	next *node[T]
	prev *node[T]
}

// ttl == 0 means not using expire time
func New[T any](maxcap int64, ttl time.Duration) *LruCache[T] {
	if maxcap <= 0 || ttl < 0 {
		return nil
	}
	return &LruCache[T]{
		maxcap: maxcap,
		ttl:    ttl,
		buf:    make(map[any]*node[T], int(float64(maxcap)*float64(1.3))),
		pool:   &sync.Pool{},
	}
}
func (l *LruCache[T]) refresh(v *node[T]) {
	if l.head == l.tail || v.prev == nil {
		//this is the only one element in this lru
		//or this element is the head
		//nothing need to do any more
		return
	}
	//this element is not the head
	//remove the element first
	v.prev.next = v.next
	if v.next == nil {
		//this element is the tail
		//after remove,it's prev will be the tail
		l.tail = v.prev
	} else {
		//this element is not the tail
		//is't next need to point to it's prev
		v.next.prev = v.prev
	}
	v.prev = nil
	v.next = l.head
	l.head.prev = v
	l.head = v
}
func (l *LruCache[T]) insert(v *node[T]) {
	//new node always put at the head
	v.next = l.head
	if l.head == nil {
		//no element in thie lru,this new node is the head
		l.head = v
	} else {
		l.head.prev = v
		l.head = v
	}
	if l.tail == nil {
		//no element in the lru,this new node is the head and the tail
		l.tail = l.head
	}
	if l.curcap == l.maxcap {
		//this lru is full,delete the tail
		delete(l.buf, l.tail.data.key)
		temp := l.tail
		l.tail = l.tail.prev
		l.tail.next = nil
		l.putPool(temp)
	} else {
		l.curcap++
	}
}
func (l *LruCache[T]) Get(key any) (data T, ok bool) {
	v, ok := l.buf[key]
	if !ok {
		return
	}
	if l.ttl != 0 {
		now := time.Now()
		if v.data.ttl <= now.UnixNano() {
			//timeout
			return
		}
		v.data.ttl = now.Add(l.ttl).UnixNano()
	}
	l.refresh(v)
	return v.data.value, true
}
func (l *LruCache[T]) Set(key any, value T) {
	if v, ok := l.buf[key]; ok {
		v.data.value = value
		if l.ttl > 0 {
			v.data.ttl = time.Now().Add(l.ttl).UnixNano()
		}
		l.refresh(v)
	} else {
		v := l.getPool()
		v.data.key = key
		v.data.value = value
		if l.ttl > 0 {
			v.data.ttl = time.Now().Add(l.ttl).UnixNano()
		}
		l.buf[key] = v
		l.insert(v)
	}
}
func (l *LruCache[T]) getPool() *node[T] {
	n, ok := l.pool.Get().(*node[T])
	if !ok {
		n = &node[T]{
			data: &kv[T]{},
		}
	}
	return n
}
func (l *LruCache[T]) putPool(n *node[T]) {
	n.data.ttl = 0
	n.data.key = ""
	var tmp T
	n.data.value = tmp
	n.next = nil
	n.prev = nil
	l.pool.Put(n)
}
