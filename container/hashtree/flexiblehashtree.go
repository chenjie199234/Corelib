package hashtree

import (
	"errors"
	"hash"
)

// thread unsafe
// node's data can't be nil and Hstr must not empty
type FlexibleHashtree[T any] hashtree[T]

var ErrMissingHstr = errors.New("missing hash str")

func NewFlexibleHashtree[T any](h hash.Hash, width int) *FlexibleHashtree[T] {
	instance := &hashtree[T]{
		encoder: h,
		width:   width,
		nodes:   make([]*node[T], 0, 100),
	}
	return (*FlexibleHashtree[T])(instance)
}

func (h *FlexibleHashtree[T]) GetNodeNum() int {
	return ((*hashtree[T])(h)).NodeNum()
}

// return nil,means this is an empty tree
func (h *FlexibleHashtree[T]) GetRootHash() []byte {
	return ((*hashtree[T])(h)).RootHash()
}
func (h *FlexibleHashtree[T]) Export() []*LeafData[T] {
	return ((*hashtree[T])(h)).Export()
}
func (h *FlexibleHashtree[T]) Reset() {
	origin := (*hashtree[T])(h)
	origin.nodes = make([]*node[T], 0, 100)
}
func (h *FlexibleHashtree[T]) Rebuild(datas []*LeafData[T]) error {
	for _, data := range datas {
		if data == nil || len(data.Hstr) == 0 {
			return ErrMissingHstr
		}
	}
	origin := (*hashtree[T])(h)
	origin.nodes = make([]*node[T], 0, len(datas))
	for _, data := range datas {
		origin.nodes = append(origin.nodes, &node[T]{
			hstr: nil,
			data: data,
		})
	}
	origin.UpdateAll()
	return nil
}
func (h *FlexibleHashtree[T]) PushSingle(data *LeafData[T]) error {
	if data == nil || len(data.Hstr) == 0 {
		return ErrMissingHstr
	}
	origin := (*hashtree[T])(h)
	newnode := &node[T]{
		hstr: nil,
		data: data,
	}
	origin.nodes = append(origin.nodes, newnode)
	origin.ReCaculateSingle(len(origin.nodes) - 1)
	return nil
}
func (h *FlexibleHashtree[T]) PushMulti(datas []*LeafData[T]) error {
	if len(datas) == 0 {
		return nil
	}
	for _, data := range datas {
		if data == nil || len(data.Hstr) == 0 {
			return ErrMissingHstr
		}
	}
	origin := (*hashtree[T])(h)
	indexes := make([]int, 0, len(datas))
	for _, data := range datas {
		newnode := &node[T]{
			hstr: nil,
			data: data,
		}
		origin.nodes = append(origin.nodes, newnode)
		indexes = append(indexes, len(origin.nodes)-1)
	}
	origin.ReCaculateMulti(indexes)
	return nil
}

// return nil means index out of range
func (h *FlexibleHashtree[T]) GetSingle(index int) *LeafData[T] {
	origin := (*hashtree[T])(h)
	if index < 0 || index >= origin.NodeNum() {
		return nil
	}
	return origin.nodes[index].data
}

// return map's value is nil means index(map's key) out of range
func (h *FlexibleHashtree[T]) GetMulti(indexes []int) map[int]*LeafData[T] {
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
func (h *FlexibleHashtree[T]) Different(other *FlexibleHashtree[T]) []int {
	originself := (*hashtree[T])(h)
	originother := (*hashtree[T])(other)
	return originself.Different(originother)
}
