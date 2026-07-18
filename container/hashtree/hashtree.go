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

// thread unsafe
type hashtree[T any] struct {
	encoder hash.Hash
	width   int //must > 0
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
	if h.NodeNum() == 0 {
		return nil
	}
	return h.nodes[0].hstr
}
func (h *hashtree[T]) ReCalculateSingle(index int) {
	if index < 0 || index >= h.NodeNum() {
		return
	}
	for {
		h.nodes[index].hstr = h.Calculate(index)
		if index == 0 {
			break
		}
		index = h.ParentIndex(index)
	}
}
func (h *hashtree[T]) ReCalculateMulti(indexes []int) {
	haszero := false
	undup := make(map[int]*struct{}, len(indexes))
	for _, index := range indexes {
		if index > 0 && index < h.NodeNum() {
			undup[index] = nil
		} else if index == 0 {
			haszero = true
		}
	}
	indexes = make([]int, 0, len(indexes))
	for k := range undup {
		indexes = append(indexes, k)
	}
	if len(indexes) == 0 {
		if haszero {
			h.nodes[0].hstr = h.Calculate(0)
		}
		return
	}
	if len(indexes) == 1 {
		h.ReCalculateSingle(indexes[0])
		return
	}
	added := make([]int, 0, 10)
	for _, index := range indexes {
		firstp := h.ParentIndex(index)
		if _, ok := undup[firstp]; ok {
			continue
		}
		added = append(added, firstp)
		undup[firstp] = nil
		if firstp > 0 {
			allp := h.AllParentIndex(firstp)
			for _, pindex := range allp {
				if _, ok := undup[pindex]; !ok {
					added = append(added, pindex)
					undup[pindex] = nil
				}
			}
		}
	}
	indexes = append(indexes, added...)
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i] > indexes[j]
	})
	for _, index := range indexes {
		h.nodes[index].hstr = h.Calculate(index)
	}
}

// reuse the hstr cache(don't care about it's correct or not)
func (h *hashtree[T]) Calculate(index int) []byte {
	if index >= len(h.nodes) {
		return nil
	}
	childstart := h.StartIndexInChildPiece(index)
	piece := make([][]byte, 0, h.width+1)
	if h.nodes[index].data == nil {
		piece = append(piece, nil)
	} else {
		piece = append(piece, h.nodes[index].data.Hstr)
	}
	for j := range h.width {
		if childstart+j >= len(h.nodes) {
			break
		}
		if h.nodes[childstart+j].hstr == nil {
			//try to fix
			h.nodes[childstart+j].hstr = h.Calculate(childstart + j)
		}
		piece = append(piece, h.nodes[childstart+j].hstr)
	}
	h.encoder.Reset()
	//prevent second-preimage attack
	if h.StartIndexInChildPiece(index) >= h.NodeNum() {
		//this node has no children,this is a leaf node
		h.encoder.Write([]byte{0x00})
	} else {
		//this node has children,this is an internal node
		h.encoder.Write([]byte{0x01})
	}
	for _, v := range piece {
		if v == nil {
			continue
		}
		h.encoder.Write(v)
	}
	return h.encoder.Sum(nil)
}

// force refresh all node's hstr,all hstr will be Calculated again to keep it correct
func (h *hashtree[T]) UpdateAll() {
	for i := h.NodeNum() - 1; i >= 0; i-- {
		h.nodes[i].hstr = h.Calculate(i)
	}
}
func (h *hashtree[T]) Export() []*LeafData[T] {
	r := make([]*LeafData[T], 0, len(h.nodes))
	for _, node := range h.nodes {
		r = append(r, node.data)
	}
	return r
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
	if selfindex <= 0 {
		return -1
	}
	sindex := h.StartIndexInSelfPiece(selfindex)
	return (sindex - 1) / h.width
}
func (h *hashtree[T]) AllParentIndex(selfindex int) []int {
	allp := make([]int, 0, 10)
	for selfindex > 0 {
		p := h.ParentIndex(selfindex)
		allp = append(allp, p)
		selfindex = p
	}
	return allp
}
func diffleaf[T any](a *hashtree[T], b *hashtree[T]) []int {
	r := make([]int, 0, 10)
	for i := 0; ; i++ {
		if i >= a.NodeNum() && i >= b.NodeNum() {
			break
		} else if i >= a.NodeNum() || i >= b.NodeNum() {
			r = append(r, i)
			continue
		}
		anode := a.nodes[i]
		bnode := b.nodes[i]
		if anode.data != nil && bnode.data != nil {
			if !bytes.Equal(anode.data.Hstr, bnode.data.Hstr) {
				r = append(r, i)
			}
		} else if anode.data != nil || bnode.data != nil {
			r = append(r, i)
		}
	}
	return r
}
