package hashtree

import (
	"errors"
	"hash"
	"math"
)

// thread unsafe
// node's data can be nill
type FixedHashtree[T any] hashtree[T]

var ErrLengthConflict = errors.New("length conflict")

func NewFixedHashtree[T any](h hash.Hash, width, tall int) *FixedHashtree[T] {
	if tall == 0 || width == 0 {
		return nil
	}
	total := 0
	for i := 0; i < tall; i++ {
		total += int(math.Pow(float64(width), float64(i)))
	}
	instance := &hashtree[T]{
		encoder: h,
		width:   width,
		nodes:   make([]*node[T], total),
	}
	for i := range instance.nodes {
		instance.nodes[i] = &node[T]{
			hstr: nil,
			data: nil,
		}
	}
	instance.UpdateAll()
	return (*FixedHashtree[T])(instance)
}
func (h *FixedHashtree[T]) GetNodeNum() int {
	return ((*hashtree[T])(h)).NodeNum()
}
func (h *FixedHashtree[T]) GetRootHash() []byte {
	return ((*hashtree[T])(h)).RootHash()
}
func (h *FixedHashtree[T]) Export() []*LeafData[T] {
	return ((*hashtree[T])(h)).Export()
}
func (h *FixedHashtree[T]) Reset() {
	origin := (*hashtree[T])(h)
	for _, node := range origin.nodes {
		node.hstr = nil
		node.data = nil
	}
	origin.UpdateAll()
}
func (h *FixedHashtree[T]) ResetSingle(index int) {
	origin := (*hashtree[T])(h)
	if index >= origin.NodeNum() || index < 0 {
		//jump the out of range index
		return
	}
	origin.nodes[index].hstr = nil
	origin.nodes[index].data = nil
	origin.ReCaculateSingle(index)
}
func (h *FixedHashtree[T]) ResetMulti(indexes []int) {
	origin := (*hashtree[T])(h)
	for _, index := range indexes {
		if index >= origin.NodeNum() || index < 0 {
			//jump the out of range index
			continue
		}
		origin.nodes[index].hstr = nil
		origin.nodes[index].data = nil
	}
	origin.ReCaculateMulti(indexes)
}
func (h *FixedHashtree[T]) Rebuild(datas []*LeafData[T]) error {
	origin := (*hashtree[T])(h)
	if len(origin.nodes) != len(datas) {
		return ErrLengthConflict
	}
	for i, data := range datas {
		origin.nodes[i].hstr = nil
		origin.nodes[i].data = data
	}
	origin.UpdateAll()
	return nil
}
func (h *FixedHashtree[T]) SetSingle(index int, data *LeafData[T]) {
	origin := (*hashtree[T])(h)
	if index < 0 || index >= origin.NodeNum() {
		//jump the out of range index
		return
	}
	origin.nodes[index].data = data
	origin.ReCaculateSingle(index)
}
func (h *FixedHashtree[T]) SetMulti(datas map[int]*LeafData[T]) {
	if len(datas) == 0 {
		return
	}
	origin := (*hashtree[T])(h)
	indexes := make([]int, 0, len(datas))
	for index, data := range datas {
		if index < 0 || index >= origin.NodeNum() {
			//jump the out of range index
			continue
		}
		origin.nodes[index].data = data
		indexes = append(indexes, index)
	}
	origin.ReCaculateMulti(indexes)
}

func (h *FixedHashtree[T]) GetSingle(index int) *LeafData[T] {
	origin := (*hashtree[T])(h)
	if index < 0 || index >= origin.NodeNum() {
		return nil
	}
	return origin.nodes[index].data
}
func (h *FixedHashtree[T]) GetMulti(indexes []int) map[int]*LeafData[T] {
	r := make(map[int]*LeafData[T], len(indexes))
	origin := (*hashtree[T])(h)
	for _, index := range indexes {
		if index < 0 || index >= origin.NodeNum() {
			r[index] = nil
		} else {
			r[index] = origin.nodes[index].data
		}
	}
	return r
}
func (h *FixedHashtree[T]) Different(other *FixedHashtree[T]) ([]int, error) {
	originself := (*hashtree[T])(h)
	originother := (*hashtree[T])(other)
	if originself.NodeNum() != originother.NodeNum() {
		return nil, ErrLengthConflict
	}
	return originself.Different(originother), nil
}
