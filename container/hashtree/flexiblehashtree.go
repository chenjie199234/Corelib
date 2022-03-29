/*
|                                 width = 3
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
|       >-------------------
|       |             |  |  |
|     >-+-----------  |  |  |
|     | |       | | | |  |  |
|   >-+-+-----  | | | |  |  |
|   ↑ ↑ ↑ | | | | | | |  |  |
| 0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16......
| ↓ | | | ↓                     |  |  |
| >-----  >---------------------------
*/

package hashtree

import (
	"bytes"
	"crypto/md5"
	"hash"
)

// thread unsafe
type FlexibleHashtree[T any] struct {
	encoder hash.Hash
	width   int
	nodes   []*flexiblenode[T]
}

//data is not empty,this is a leaf
//data is empty,this is a node
type flexiblenode[T any] struct {
	hstr []byte
	data *LeafData[T]
}

func (h *FlexibleHashtree[T]) caculate(pindex int) []byte {
	piece := make([][]byte, h.width+1)
	piece[0] = h.nodes[pindex].data.Hstr
	childstart := getNodeStartIndexInChildPiece(pindex, h.width)
	for j := 0; j < h.width; j++ {
		if childstart+j >= len(h.nodes) {
			break
		}
		if h.nodes[childstart+j].hstr == nil {
			piece[j+1] = h.nodes[childstart+j].data.Hstr
		} else {
			piece[j+1] = h.nodes[childstart+j].hstr
		}
	}
	h.encoder.Reset()
	h.encoder.Write(bytes.Join(piece, nil))
	return h.encoder.Sum(nil)
}
func NewFlexibleHashtree[T any](width int) *FlexibleHashtree[T] {
	instance := &FlexibleHashtree[T]{
		encoder: md5.New(),
		width:   width,
		nodes:   make([]*flexiblenode[T], 0, 100),
	}
	return instance
}
func (h *FlexibleHashtree[T]) UpdateAll() {
	for i := len(h.nodes) - 1; i >= 0; i -= h.width {
		pindex := getparentindex(i, h.width)
		h.nodes[pindex].hstr = h.caculate(pindex)
	}
}

//return nil,means this is an empty tree
func (h *FlexibleHashtree[T]) GetRootHash() []byte {
	if len(h.nodes) == 0 {
		return nil
	}
	return h.nodes[0].hstr
}
func (h *FlexibleHashtree[T]) Reset() {
	h.nodes = make([]*flexiblenode[T], 0)
}
func (h *FlexibleHashtree[T]) Rebuild(datas []*LeafData[T]) {
	h.nodes = make([]*flexiblenode[T], 0, len(datas))
	for _, data := range datas {
		h.nodes = append(h.nodes, &flexiblenode[T]{
			hstr: nil,
			data: data,
		})
	}
	h.UpdateAll()
}
func (h *FlexibleHashtree[T]) Insert(data *LeafData[T]) {
	newnode := &flexiblenode[T]{
		hstr: nil,
		data: data,
	}
	h.nodes = append(h.nodes, newnode)
	pindex := getparentindex(len(h.nodes)-1, h.width)
	for {
		h.nodes[pindex].hstr = h.caculate(pindex)
		if pindex == 0 {
			break
		}
		pindex = getparentindex(pindex, h.width)
	}
}
func (h *FlexibleHashtree[T]) Export() []*LeafData[T] {
	r := make([]*LeafData[T], 0, len(h.nodes))
	for _, node := range h.nodes {
		r = append(r, node.data)
	}
	return r
}
