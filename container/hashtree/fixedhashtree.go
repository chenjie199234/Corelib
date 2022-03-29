/*
|                        width = 3,tall = 4
|                               N(hstr)                      |
|                ---------------|------------------
|                |              |                 |          t
|                N(hstr)        N(hstr)           N(hstr)
|       ---------|---------     |                 |          a
|       |        |        |    ...               ...
|       N(hstr)  N(hstr)  N(hstr)                            l
| ------|------  |        |
| |     |     | ...      ...                                 l
| N(L)  N(L)  N(L) 
|(hstr and data)                                             |
|
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
	"errors"
	"hash"
	"math"
)

var (
	ErrLeafIndexOutOfRange = errors.New("leaf index out of range")
	ErrLeafLength          = errors.New("leaf length not equal")
)

//thread unsafe
type FixedHashtree[T any] struct {
	encoder hash.Hash
	nodes   []*fixednode[T]
	leaves  []*fixednode[T]
	width   int
	tall    int
}
type fixednode[T any] struct {
	data      *LeafData[T]
	nodeindex int
	leafindex int
}

func (h *FixedHashtree[T]) caculate(pindex int) []byte {
	piece := make([][]byte, h.width)
	childstart := getNodeStartIndexInChildPiece(pindex, h.width)
	for j := 0; j < h.width; j++ {
		if childstart+j >= len(h.nodes) {
			break
		}
		piece[j] = h.nodes[childstart+j].data.Hstr
	}
	h.encoder.Reset()
	h.encoder.Write(bytes.Join(piece, nil))
	return h.encoder.Sum(nil)
}

func NewFixedHashtree[T any](width, tall int) *FixedHashtree[T] {
	if tall == 0 || width == 0 {
		return nil
	}
	nodesnum := 0
	for i := 1; i <= tall; i++ {
		nodesnum += int(math.Pow(float64(width), float64(i-1)))
	}
	leavesnum := int(math.Pow(float64(width), float64(tall-1)))
	instance := &FixedHashtree[T]{
		encoder: md5.New(),
		nodes:   make([]*fixednode[T], nodesnum),
		width:   width,
		tall:    tall,
	}
	instance.leaves = instance.nodes[len(instance.nodes)-leavesnum:]
	for i := range instance.nodes {
		var tmp T
		instance.nodes[i] = &fixednode[T]{
			data:      &LeafData[T]{nil, tmp},
			nodeindex: i,
			leafindex: -1,
		}
	}
	for i := range instance.leaves {
		instance.leaves[i].leafindex = i
	}
	instance.UpdateAll()
	return instance
}
func (h *FixedHashtree[T]) UpdateAll() {
	for i := len(h.nodes) - 1; i >= 0; i -= h.width {
		pindex := getparentindex(i, h.width)
		h.nodes[pindex].data.Hstr = h.caculate(pindex)
	}
}
func (h *FixedHashtree[T]) GetLeavesNum() int {
	return len(h.leaves)
}
func (h *FixedHashtree[T]) GetRootHash() []byte {
	return h.nodes[0].data.Hstr
}
func (h *FixedHashtree[T]) Reset() {
	for i := range h.leaves {
		h.leaves[i].data.Hstr = nil
		var tmp T
		h.leaves[i].data.Value = tmp
	}
	h.UpdateAll()
}
func (h *FixedHashtree[T]) ResetSingleLeaf(leafindex int) error {
	return h.SetSingleLeaf(leafindex, nil)
}
func (h *FixedHashtree[T]) ResetMultiLeaves(leafindexs []int) error {
	req := make(map[int]*LeafData[T])
	for _, v := range leafindexs {
		req[v] = nil
	}
	return h.SetMultiLeaves(req)
}
func (h *FixedHashtree[T]) Rebuild(datas map[int]*LeafData[T]) error {
	for leafindex := range datas {
		if leafindex >= len(h.leaves) {
			return ErrLeafIndexOutOfRange
		}
	}
	for i := range h.leaves {
		h.leaves[i].data.Hstr = nil
		var tmp T
		h.leaves[i].data.Value = tmp
	}
	for leafindex, leafdata := range datas {
		if leafdata != nil {
			h.leaves[leafindex].data.Hstr = leafdata.Hstr
			h.leaves[leafindex].data.Value = leafdata.Value
		}
	}
	h.UpdateAll()
	return nil
}
func (h *FixedHashtree[T]) SetSingleLeaf(leafindex int, data *LeafData[T]) error {
	if leafindex >= len(h.leaves) {
		return ErrLeafIndexOutOfRange
	}
	if data == nil {
		h.leaves[leafindex].data.Hstr = nil
		var tmp T
		h.leaves[leafindex].data.Value = tmp
	} else {
		h.leaves[leafindex].data.Hstr = data.Hstr
		h.leaves[leafindex].data.Value = data.Value
	}
	pindex := getparentindex(h.leaves[leafindex].nodeindex, h.width)
	for {
		h.nodes[pindex].data.Hstr = h.caculate(pindex)
		if pindex == 0 {
			break
		}
		pindex = getparentindex(pindex, h.width)
	}
	return nil
}
func (h *FixedHashtree[T]) SetMultiLeaves(datas map[int]*LeafData[T]) error {
	if len(datas) == 0 {
		return nil
	}
	for leafindex := range datas {
		if leafindex >= len(h.leaves) {
			return ErrLeafIndexOutOfRange
		}
	}
	pindexs := make(map[int]*struct{}, 10)
	for leafindex, leafdata := range datas {
		if leafdata == nil {
			h.leaves[leafindex].data.Hstr = nil
			var tmp T
			h.leaves[leafindex].data.Value = tmp
		} else {
			h.leaves[leafindex].data.Hstr = leafdata.Hstr
			h.leaves[leafindex].data.Value = leafdata.Value
		}
		pindexs[getparentindex(h.leaves[leafindex].nodeindex, h.width)] = nil
	}
	finish := false
	for {
		newpindex := make(map[int]*struct{}, 0)
		for pindex := range pindexs {
			h.nodes[pindex].data.Hstr = h.caculate(pindex)
			if pindex == 0 {
				finish = true
				break
			}
			newpindex[getparentindex(pindex, h.width)] = nil
		}
		if finish {
			break
		}
		pindexs = newpindex
	}
	return nil
}
func (h *FixedHashtree[T]) Export() map[int]*LeafData[T] {
	result := make(map[int]*LeafData[T])
	for leafindex, leafdata := range h.leaves {
		result[leafindex] = &LeafData[T]{
			Hstr:  leafdata.data.Hstr,
			Value: leafdata.data.Value,
		}
	}
	return result
}
func (h *FixedHashtree[T]) GetSingleLeaf(leafindex int) (*LeafData[T], error) {
	if leafindex >= len(h.leaves) {
		return nil, ErrLeafIndexOutOfRange
	}
	return &LeafData[T]{
		Hstr:  h.leaves[leafindex].data.Hstr,
		Value: h.leaves[leafindex].data.Value,
	}, nil
}
func (h *FixedHashtree[T]) GetMultiLeaves(leafindexs []int) (map[int]*LeafData[T], error) {
	for _, leafindex := range leafindexs {
		if leafindex >= len(h.leaves) {
			return nil, ErrLeafIndexOutOfRange
		}
	}
	result := make(map[int]*LeafData[T])
	for _, leafindex := range leafindexs {
		result[leafindex] = &LeafData[T]{
			Hstr:  h.leaves[leafindex].data.Hstr,
			Value: h.leaves[leafindex].data.Value,
		}
	}
	return result, nil
}
func (h *FixedHashtree[T]) Different(other *FixedHashtree[T]) ([]int, error) {
	if len(h.leaves) != len(other.leaves) {
		return nil, ErrLeafLength
	}
	result := make([]int, 0, 100)
	maydifindexs := make(map[int]*struct{})
	maydifindexs[0] = nil
	for {
		newmaydifindexs := make(map[int]*struct{})
		for nodeindex := range maydifindexs {
			if !bytes.Equal(h.nodes[nodeindex].data.Hstr, other.nodes[nodeindex].data.Hstr) {
				if childnodestart := getNodeStartIndexInChildPiece(nodeindex, h.width); childnodestart >= len(h.nodes) {
					result = append(result, h.nodes[nodeindex].leafindex)
				} else {
					for i := 0; i < h.width; i++ {
						newmaydifindexs[childnodestart+i] = nil
					}
				}
			}
		}
		if len(newmaydifindexs) > 0 {
			maydifindexs = newmaydifindexs
		} else {
			break
		}
	}
	return result, nil
}
