/*                                 width = 3
|                               N(hstr and data)                  |
|                -------------------|-------------------
|                |                  |                  |          t
|                N(hstr and data)   N(hstr and data)   N(hstr)
|       ---------|---------         |                  |          a
|       |        |        |        ...                ...
|       N        N        N(hstr and data)                        l
| ------|------  |        |
| |     |     | ...      ...                                      l
| N     N     N(hstr and data)
|                                                                 |
|
|       >--------------------
|       |             |  |  |
|     >-+------------ |  |  |
|     | |       | | | |  |  |
|   >-+-+-----  | | | |  |  |
|   ↑ ↑ ↑ | | | | | | |  |  |
| 0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16......
| ↓ | | | ↓                     |  |  |
| >-----  >---------------------------                           */

package hashtree

import (
	"bytes"
	"hash"
	"sort"
)

type hashtree[T any] struct {
	encoder hash.Hash
	width   int
	nodes   []*node[T]
}
type node[T any] struct {
	hstr []byte
	data *LeafData[T]
}
type LeafData[T any] struct {
	Hstr  []byte
	Value T
}

func (h *hashtree[T]) NodeNum() int {
	return len(h.nodes)
}
func (h *hashtree[T]) RootHash() []byte {
	if len(h.nodes) == 0 {
		return nil
	}
	if h.nodes[0].hstr != nil {
		return h.nodes[0].hstr
	}
	if h.nodes[0].data == nil {
		return nil
	}
	return h.nodes[0].data.Hstr
}
func (h *hashtree[T]) ReCaculateSingle(index int) {
	if index < 0 || index >= h.NodeNum() {
		return
	}
	for {
		h.nodes[index].hstr = h.Caculate(index)
		if index == 0 {
			break
		}
		index = h.ParentIndex(index)
	}
}
func (h *hashtree[T]) ReCaculateMulti(indexes []int) {
	sort.Ints(indexes)
	//remove too small
	for i := 0; i < len(indexes); i++ {
		if indexes[i] >= 0 {
			indexes = indexes[i:]
			break
		}
	}
	//remove too big
	for i := 1; i <= len(indexes); i++ {
		if indexes[len(indexes)-i] < h.NodeNum() {
			indexes = indexes[:len(indexes)-i]
			break
		}
	}
	if len(indexes) == 0 {
		return
	}
	if len(indexes) == 1 {
		h.ReCaculateSingle(indexes[0])
		return
	}
	undup := make(map[int]*struct{}, len(indexes))
	for _, index := range indexes {
		undup[index] = nil
	}
	for len(indexes) > 0 {
		if len(indexes) == 1 {
			h.ReCaculateSingle(indexes[0])
			break
		}
		curp := -1
		for i := 1; i <= len(indexes); {
			//some index may have the same parent
			index := indexes[len(indexes)-i]
			pindex := h.ParentIndex(index)
			if curp != -1 && curp != pindex {
				//this node belong's to another parent
				indexes = indexes[:len(indexes)-i+1]
				break
			}
			h.nodes[index].hstr = h.Caculate(index)
			if curp == -1 {
				curp = h.ParentIndex(index)
			}
			delete(undup, index)
		}
		if _, ok := undup[curp]; !ok {
			indexes = append(indexes, curp)
			sort.Ints(indexes)
			undup[curp] = nil
		}
	}
}
func (h *hashtree[T]) Caculate(index int) []byte {
	childstart := h.StartIndexInChildPiece(index)
	if childstart >= len(h.nodes) {
		//this node has no children,don't need to caculate,use it's data's Hstr
		return nil
	}
	piece := make([][]byte, 0, h.width+1)
	if h.nodes[index].data == nil {
		piece = append(piece, nil)
	} else {
		piece = append(piece, h.nodes[index].data.Hstr)
	}
	for j := 0; j < h.width; j++ {
		if childstart+j >= len(h.nodes) {
			break
		}
		if h.nodes[childstart+j].hstr != nil {
			piece = append(piece, h.nodes[childstart+j].hstr)
		} else if h.nodes[childstart+j].data != nil {
			piece = append(piece, h.nodes[childstart+j].data.Hstr)
		} else {
			piece = append(piece, nil)
		}
	}
	h.encoder.Reset()
	h.encoder.Write(bytes.Join(piece, nil))
	return h.encoder.Sum(nil)
}
func (h *hashtree[T]) UpdateAll() {
	for i := len(h.nodes) - 1; i >= 0; i -= h.width {
		pindex := h.ParentIndex(i)
		h.nodes[pindex].hstr = h.Caculate(pindex)
		if pindex == 0 {
			break
		}
	}
}
func (h *hashtree[T]) Export() []*LeafData[T] {
	r := make([]*LeafData[T], 0, len(h.nodes))
	for _, node := range h.nodes {
		r = append(r, node.data)
	}
	return r
}
func (h *hashtree[T]) Different(o *hashtree[T]) []int {
	var less *hashtree[T]
	if len(h.nodes) < len(o.nodes) {
		less = h
	} else {
		less = o
	}
	result := make([]int, 0, 10)
	for i := range less.nodes {
		hnode := h.nodes[i]
		onode := o.nodes[i]
		if (hnode.data == nil || len(hnode.data.Hstr) == 0) && (onode.data == nil || len(onode.data.Hstr) == 0) {
			continue
		}
		if hnode.data != nil && onode.data != nil && bytes.Equal(hnode.data.Hstr, onode.data.Hstr) {
			continue
		}
		result = append(result, i)
	}
	if less == h {
		for i := len(h.nodes); i < len(o.nodes); i++ {
			result = append(result, i)
		}
	} else {
		for i := len(o.nodes); i < len(h.nodes); i++ {
			result = append(result, i)
		}
	}
	return result
}
func (h *hashtree[T]) StartIndexInSelfPiece(selfindex int) int {
	if selfindex == 0 {
		return 0
	}
	return (((selfindex-1)/h.width)*h.width + 1)
}
func (h *hashtree[T]) StartIndexInChildPiece(selfindex int) int {
	return selfindex*h.width + 1
}
func (h *hashtree[T]) ParentIndex(selfindex int) int {
	if selfindex == 0 {
		return -1
	}
	sindex := h.StartIndexInSelfPiece(selfindex)
	return (sindex - 1) / h.width
}
