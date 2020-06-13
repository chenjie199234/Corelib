//the order in data,left = index * 2 + 1,right = (index + 1) * 2
//          0
//   1             2
// 3   4        5     6
//7 8 9 10    11 12 13 14

package heap

import (
	"sync"
	"unsafe"
)

func NewHeapMaxNum(minbufnum int64) *HeapNum {
	return &HeapNum{
		direction: 1,
		data:      make([]*heapNodeNum, 0, minbufnum),
		pool: &sync.Pool{
			New: func() interface{} {
				return &heapNodeNum{}
			},
		},
	}
}
func NewHeapMinNum(minbufnum int64) *HeapNum {
	return &HeapNum{
		direction: 0,
		data:      make([]*heapNodeNum, 0, minbufnum),
		pool: &sync.Pool{
			New: func() interface{} {
				return &heapNodeNum{}
			},
		},
	}
}
func NewHeapMaxStr(minbufnum int64) *HeapStr {
	return &HeapStr{
		direction: 1,
		data:      make([]*heapNodeStr, 0, minbufnum),
		pool: &sync.Pool{
			New: func() interface{} {
				return &heapNodeStr{}
			},
		},
	}
}
func NewHeapMinStr(minbufnum int64) *HeapStr {
	return &HeapStr{
		direction: 0,
		data:      make([]*heapNodeStr, 0, minbufnum),
		pool: &sync.Pool{
			New: func() interface{} {
				return &heapNodeStr{}
			},
		},
	}
}

type HeapNum struct {
	sync.RWMutex
	//1-for max,0 for min
	direction int64
	curnum    int64
	data      []*heapNodeNum
	pool      *sync.Pool
}
type heapNodeNum struct {
	key   int64
	value unsafe.Pointer
}

func (this *HeapNum) Insert(key int64, value unsafe.Pointer) {
	if value == nil {
		return
	}
	this.Lock()
	this.data = append(this.data, this.getnode(key, value))
	newindex := this.curnum
	this.curnum++
	if newindex == 0 {
		//this is root
		this.Unlock()
		return
	}
	for {
		var parentindex int64
		if newindex%2 > 0 {
			parentindex = (newindex - 1) / 2
		} else {
			parentindex = newindex/2 - 1
		}
		if this.direction == 1 {
			//this is a max heap
			if this.data[parentindex].key < key {
				this.data[parentindex], this.data[newindex] = this.data[newindex], this.data[parentindex]
			} else {
				break
			}
		} else {
			//this is a min heap
			if this.data[parentindex].key > key {
				this.data[parentindex], this.data[newindex] = this.data[newindex], this.data[parentindex]
			} else {
				break
			}
		}
		if parentindex == 0 {
			break
		}
		newindex = parentindex
	}
	this.Unlock()
}

//only get not delete
func (this *HeapNum) Getroot() (key int64, value unsafe.Pointer) {
	this.RLock()
	if this.curnum == 0 {
		key = 0
		value = nil
	} else {
		key, value = this.data[0].key, this.data[0].value
	}
	this.RUnlock()
	return
}

//get and delete
func (this *HeapNum) Poproot() (key int64, value unsafe.Pointer) {
	this.Lock()
	if this.curnum == 0 {
		key = 0
		value = nil
	} else {
		//get data
		key, value = this.data[0].key, this.data[0].value
		//delete node
		this.putnode(this.data[0])
		this.curnum--
		if this.curnum == 0 {
			this.data = this.data[:0]
		} else {
			//rebuild
			this.data[0] = this.data[len(this.data)-1]
			this.data = this.data[:len(this.data)-1]
			newindex := 0
			for {
				leftindex := newindex*2 + 1
				rightindex := (newindex + 1) * 2
				if this.direction == 1 {
					//this is a max heap
					maxindex := -1
					if leftindex <= len(this.data)-1 {
						//valid index
						if this.data[newindex].key >= this.data[leftindex].key {
							maxindex = newindex
						} else {
							maxindex = leftindex
						}
					}
					if rightindex <= len(this.data)-1 {
						//valid index
						if this.data[maxindex].key >= this.data[rightindex].key {
							//maxindex = maxindex
						} else {
							maxindex = rightindex
						}
					}
					if newindex == maxindex || maxindex == -1 {
						break
					}
					this.data[newindex], this.data[maxindex] = this.data[maxindex], this.data[newindex]
					newindex = maxindex
				} else {
					//this is a min heap
					minindex := -1
					if leftindex <= len(this.data)-1 {
						//valid index
						if this.data[newindex].key <= this.data[leftindex].key {
							minindex = newindex
						} else {
							minindex = leftindex
						}
					}
					if rightindex <= len(this.data)-1 {
						//valid index
						if this.data[minindex].key <= this.data[rightindex].key {
							//minindex = minindex
						} else {
							minindex = rightindex
						}
					}
					if newindex == minindex || minindex == -1 {
						break
					}
					this.data[newindex], this.data[minindex] = this.data[minindex], this.data[newindex]
					newindex = minindex
				}
			}
		}
	}
	this.Unlock()
	return
}
func (this *HeapNum) getnode(key int64, value unsafe.Pointer) *heapNodeNum {
	node, _ := this.pool.Get().(*heapNodeNum)
	node.key = key
	node.value = value
	return node
}
func (this *HeapNum) putnode(node *heapNodeNum) {
	node.key = 0
	node.value = nil
	this.pool.Put(node)
}

type HeapStr struct {
	sync.RWMutex
	//1-for max,0 for min
	direction int64
	curnum    int64
	data      []*heapNodeStr
	pool      *sync.Pool
}
type heapNodeStr struct {
	key   string
	value unsafe.Pointer
}

func (this *HeapStr) Insert(key string, value unsafe.Pointer) {
	if value == nil {
		return
	}
	this.Lock()
	this.data = append(this.data, this.getnode(key, value))
	newindex := this.curnum
	this.curnum++
	if newindex == 0 {
		//this is root
		this.Unlock()
		return
	}
	for {
		var parentindex int64
		if newindex%2 > 0 {
			parentindex = (newindex - 1) / 2
		} else {
			parentindex = newindex/2 - 1
		}
		if this.direction == 1 {
			//this is a max heap
			if this.data[parentindex].key < key {
				this.data[parentindex], this.data[newindex] = this.data[newindex], this.data[parentindex]
			} else {
				break
			}
		} else {
			//this is a min heap
			if this.data[parentindex].key > key {
				this.data[parentindex], this.data[newindex] = this.data[newindex], this.data[parentindex]
			} else {
				break
			}
		}
		if parentindex == 0 {
			break
		}
		newindex = parentindex
	}
	this.Unlock()
}

//only get not delete
func (this *HeapStr) Getroot() (key string, value unsafe.Pointer) {
	this.RLock()
	if this.curnum == 0 {
		key = ""
		value = nil
	} else {
		key, value = this.data[0].key, this.data[0].value
	}
	this.RUnlock()
	return
}

//get and delete
func (this *HeapStr) Poproot() (key string, value unsafe.Pointer) {
	this.Lock()
	if this.curnum == 0 {
		key = ""
		value = nil
	} else {
		//get data
		key, value = this.data[0].key, this.data[0].value
		//delete node
		this.putnode(this.data[0])
		this.curnum--
		if this.curnum == 0 {
			this.data = this.data[:0]
		} else {
			//rebuild
			this.data[0] = this.data[len(this.data)-1]
			this.data = this.data[:len(this.data)-1]
			newindex := 0
			for {
				leftindex := newindex*2 + 1
				rightindex := (newindex + 1) * 2
				if this.direction == 1 {
					//this is a max heap
					maxindex := -1
					if leftindex <= len(this.data)-1 {
						//valid index
						if this.data[newindex].key >= this.data[leftindex].key {
							maxindex = newindex
						} else {
							maxindex = leftindex
						}
					}
					if rightindex <= len(this.data)-1 {
						//valid index
						if this.data[maxindex].key >= this.data[rightindex].key {
							//maxindex = maxindex
						} else {
							maxindex = rightindex
						}
					}
					if newindex == maxindex || maxindex == -1 {
						break
					}
					this.data[newindex], this.data[maxindex] = this.data[maxindex], this.data[newindex]
					newindex = maxindex
				} else {
					//this is a min heap
					minindex := -1
					if leftindex <= len(this.data)-1 {
						//valid index
						if this.data[newindex].key <= this.data[leftindex].key {
							minindex = newindex
						} else {
							minindex = leftindex
						}
					}
					if rightindex <= len(this.data)-1 {
						//valid index
						if this.data[minindex].key <= this.data[rightindex].key {
							//minindex = minindex
						} else {
							minindex = rightindex
						}
					}
					if newindex == minindex || minindex == -1 {
						break
					}
					this.data[newindex], this.data[minindex] = this.data[minindex], this.data[newindex]
					newindex = minindex
				}
			}
		}
	}
	this.Unlock()
	return
}
func (this *HeapStr) getnode(key string, value unsafe.Pointer) *heapNodeStr {
	node, _ := this.pool.Get().(*heapNodeStr)
	node.key = key
	node.value = value
	return node
}
func (this *HeapStr) putnode(node *heapNodeStr) {
	node.key = ""
	node.value = nil
	this.pool.Put(node)
}
