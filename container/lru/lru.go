package lru

import (
	"sync"
	"time"
	"unsafe"
)

//thread unsafe
type LruCache struct {
	maxcap int64
	curcap int64
	ttl    time.Duration
	buf    map[interface{}]*node
	head   *node
	tail   *node
	pool   *sync.Pool
}
type kv struct {
	ttl   int64 //unixnano
	key   interface{}
	value unsafe.Pointer
}

type node struct {
	data *kv
	next *node
	prev *node
}

//ttl == 0 means not using expire time
func New(maxcap int64, ttl time.Duration) *LruCache {
	if maxcap <= 0 || ttl < 0 {
		return nil
	}
	return &LruCache{
		maxcap: maxcap,
		ttl:    ttl,
		buf:    make(map[interface{}]*node, int(float64(maxcap)*float64(1.3))),
		pool:   &sync.Pool{},
	}
}
func (l *LruCache) refresh(v *node) {
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
func (l *LruCache) insert(v *node) {
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
func (l *LruCache) Get(key interface{}) (unsafe.Pointer, bool) {
	v, ok := l.buf[key]
	if !ok {
		return nil, false
	}
	if l.ttl != 0 {
		now := time.Now()
		if v.data.ttl <= now.UnixNano() {
			//timeout
			return nil, false
		}
		v.data.ttl = now.Add(l.ttl).UnixNano()
	}
	l.refresh(v)
	return v.data.value, true
}
func (l *LruCache) Set(key interface{}, value unsafe.Pointer) {
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
func (l *LruCache) getPool() *node {
	n, ok := l.pool.Get().(*node)
	if !ok {
		n = &node{
			data: &kv{},
		}
	}
	return n
}
func (l *LruCache) putPool(n *node) {
	n.data.ttl = 0
	n.data.key = ""
	n.data.value = nil
	n.next = nil
	n.prev = nil
	l.pool.Put(n)
}
