//the order in data,left = index * 2 + 1,right = (index + 1) * 2
//          0
//   1             2
// 3   4        5     6
//7 8 9 10    11 12 13 14

package heap

// thread unsafe
type Heap[T any] struct {
	direction func(a, b T) bool
	datas     []T
}

// direction return a > b max heap
// direction return a < b min heap
func NewHeap[T any](direction func(a, b T) bool) *Heap[T] {
	if direction == nil {
		return nil
	}
	return &Heap[T]{
		direction: direction,
		datas:     make([]T, 0, 256),
	}
}

func (this *Heap[T]) Push(data T) {
	this.datas = append(this.datas, data)
	newindex := len(this.datas) - 1
	if newindex == 0 {
		//this is root
		return
	}
	for {
		var parentindex int
		if newindex%2 > 0 {
			//newindex is the left node
			parentindex = (newindex - 1) / 2
		} else {
			//newindex is the right node
			parentindex = newindex/2 - 1
		}
		if this.direction(this.datas[parentindex], data) {
			break
		}
		this.datas[parentindex], this.datas[newindex] = this.datas[newindex], this.datas[parentindex]
		if parentindex == 0 {
			break
		}
		newindex = parentindex
	}
}

// only get not delete,return false means this is an empty heap
func (this *Heap[T]) GetRoot() (data T, ok bool) {
	if len(this.datas) > 0 {
		return this.datas[0], true
	}
	return
}

// get and delete
func (this *Heap[T]) PopRoot() (data T, ok bool) {
	if len(this.datas) == 0 {
		return
	}
	data, ok = this.datas[0], true
	if len(this.datas) <= 2 {
		this.datas = this.datas[1:]
		return
	}
	//rebuild
	this.datas[0] = this.datas[len(this.datas)-1]
	this.datas = this.datas[:len(this.datas)-1]
	newindex := 0
	for {
		leftindex := newindex*2 + 1
		rightindex := (newindex + 1) * 2
		if leftindex >= len(this.datas) {
			//lleftindex and rightindex both are invalid
			break
		}
		if rightindex < len(this.datas) {
			//leftindex and rightindex both are valid index
			//compare left and right
			if this.direction(this.datas[leftindex], this.datas[rightindex]) {
				this.datas[leftindex], this.datas[newindex] = this.datas[newindex], this.datas[leftindex]
				newindex = leftindex
			} else {
				this.datas[rightindex], this.datas[newindex] = this.datas[newindex], this.datas[rightindex]
				newindex = rightindex
			}
		} else {
			//leftindex is valid but rightindex is invalid index
			//compare left and top
			if this.direction(this.datas[leftindex], this.datas[newindex]) {
				this.datas[leftindex], this.datas[newindex] = this.datas[newindex], this.datas[leftindex]
			}
			break
		}
	}
	return
}
