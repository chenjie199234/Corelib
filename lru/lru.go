package lru

import (
	"sync"
	"time"
	"unsafe"
)

//thread unsafe
type LruCache struct {
	sync.Mutex
	maxcap int64
	ttl    int64
	curcap int64
	buf    map[string]*node
	head   *node
	tail   *node
	pool   *sync.Pool
}
type kv struct {
	ttl   int64
	key   string
	value unsafe.Pointer
}

type node struct {
	data *kv
	next *node
	prev *node
}

//ttl == 0 means not using expire time
func New(maxcap, ttl int64) *LruCache {
	if maxcap <= 0 {
		return nil
	}
	return &LruCache{
		maxcap: maxcap,
		ttl:    ttl,
		buf:    make(map[string]*node, int(float64(maxcap)*float64(1.3))),
		pool: &sync.Pool{
			New: func() interface{} {
				return &node{
					data: &kv{},
				}
			},
		},
	}
}
func (l *LruCache) Get(key string) unsafe.Pointer {
	l.Lock()
	var result unsafe.Pointer
	if v, ok := l.buf[key]; ok {
		if l.ttl == 0 || (l.ttl > 0 && v.data.ttl > time.Now().Unix()) {
			result = v.data.value
			//update
			if l.head != l.tail {
				if v.prev != nil {
					v.prev.next = v.next
					if v.next == nil {
						l.tail = v.prev
					}
				}
				if v.next != nil {
					v.next.prev = v.prev
					if v.prev == nil {
						l.head = v.next
					}
				}
				v.prev = nil
				v.next = l.head
				l.head.prev = v
				l.head = v
				if l.ttl > 0 {
					v.data.ttl = time.Now().Unix() + l.ttl
				}
			}
		}
	}
	l.Unlock()
	return result
}
func (l *LruCache) Set(key string, value unsafe.Pointer) {
	l.Lock()
	if v, ok := l.buf[key]; ok {
		v.data.value = value
		//update
		if l.head != l.tail {
			if v.prev != nil {
				v.prev.next = v.next
				if v.next == nil {
					l.tail = v.prev
				}
			}
			if v.next != nil {
				v.next.prev = v.prev
				if v.prev == nil {
					l.head = v.next
				}
			}
			v.prev = nil
			v.next = l.head
			l.head.prev = v
			l.head = v
			if l.ttl > 0 {
				v.data.ttl = time.Now().Unix() + l.ttl
			}
		}
	} else {
		v := l.getPool()
		v.data.key = key
		v.data.value = value
		if l.ttl > 0 {
			v.data.ttl = time.Now().Unix() + l.ttl
		}
		v.next = l.head
		if l.head == nil {
			l.head = v
		} else {
			l.head.prev = v
			l.head = v
		}
		if l.tail == nil {
			l.tail = l.head
		}
		l.buf[key] = v
		if l.curcap == l.maxcap {
			delete(l.buf, l.tail.data.key)
			temp := l.tail
			l.tail = l.tail.prev
			l.tail.next = nil
			l.putPool(temp)
		} else {
			l.curcap++
		}
	}
	l.Unlock()
}
func (l *LruCache) getPool() *node {
	return l.pool.Get().(*node)
}
func (l *LruCache) putPool(n *node) {
	n.data.ttl = 0
	n.data.key = ""
	n.data.value = nil
	n.next = nil
	n.prev = nil
	l.pool.Put(n)
}
