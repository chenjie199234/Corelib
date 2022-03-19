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
type FixedHashtree struct {
	encoder hash.Hash
	nodes   []*fixednode
	leaves  []*fixednode
	width   int
	tall    int
}
type fixednode struct {
	data      *LeafData
	nodeindex int
	leafindex int
}

func (h *FixedHashtree) caculate(pindex int) []byte {
	childstart := h.getNodeStartIndexInChildPiece(pindex)
	piece := make([][]byte, h.width)
	for j := 0; j < h.width; j++ {
		piece[j] = h.nodes[childstart+j].data.Hashstr
	}
	h.encoder.Reset()
	h.encoder.Write(bytes.Join(piece, nil))
	return h.encoder.Sum(nil)
}
func (h *FixedHashtree) getNodeStartIndexInSelfPiece(leafindex int) int {
	return (((leafindex + h.width - 1) / h.width) * h.width)
}
func (h *FixedHashtree) getNodeStartIndexInChildPiece(index int) int {
	return index*h.width + 1
}
func (h *FixedHashtree) getparentindex(index int) int {
	sindex := h.getNodeStartIndexInSelfPiece(index)
	return (sindex - 1) / h.width
}

func NewFixedHashtree(width, tall int) *FixedHashtree {
	if tall == 0 || width == 0 {
		return nil
	}
	nodesnum := 0
	for i := 1; i <= tall; i++ {
		nodesnum += int(math.Pow(float64(width), float64(i-1)))
	}
	leavesnum := int(math.Pow(float64(width), float64(tall-1)))
	instance := &FixedHashtree{
		encoder: md5.New(),
		nodes:   make([]*fixednode, nodesnum),
		width:   width,
		tall:    tall,
	}
	instance.leaves = instance.nodes[len(instance.nodes)-leavesnum:]
	for i := range instance.nodes {
		instance.nodes[i] = &fixednode{
			data:      &LeafData{nil, nil},
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
func (h *FixedHashtree) UpdateAll() {
	for i := len(h.nodes) - 1; i >= 0; i -= h.width {
		pindex := h.getparentindex(i)
		h.nodes[pindex].data.Hashstr = h.caculate(pindex)
	}
}
func (h *FixedHashtree) GetLeavesNum() int {
	return len(h.leaves)
}
func (h *FixedHashtree) GetRootHash() []byte {
	return h.nodes[0].data.Hashstr
}
func (h *FixedHashtree) Reset() {
	for i := range h.leaves {
		h.leaves[i].data.Hashstr = nil
		h.leaves[i].data.Value = nil
	}
	h.UpdateAll()
}
func (h *FixedHashtree) ResetSingleLeaf(leafindex int) error {
	return h.SetSingleLeaf(leafindex, nil)
}
func (h *FixedHashtree) ResetMultiLeaves(leafindexs []int) error {
	req := make(map[int]*LeafData)
	for _, v := range leafindexs {
		req[v] = nil
	}
	return h.SetMultiLeaves(req)
}
func (h *FixedHashtree) Rebuild(datas map[int]*LeafData) error {
	for leafindex := range datas {
		if leafindex >= len(h.leaves) {
			return ErrLeafIndexOutOfRange
		}
	}
	for i := range h.leaves {
		h.leaves[i].data.Hashstr = nil
		h.leaves[i].data.Value = nil
	}
	for leafindex, leafdata := range datas {
		if leafdata != nil {
			h.leaves[leafindex].data.Hashstr = leafdata.Hashstr
			h.leaves[leafindex].data.Value = leafdata.Value
		}
	}
	h.UpdateAll()
	return nil
}
func (h *FixedHashtree) SetSingleLeaf(leafindex int, data *LeafData) error {
	if leafindex >= len(h.leaves) {
		return ErrLeafIndexOutOfRange
	}
	if data == nil {
		h.leaves[leafindex].data.Hashstr = nil
		h.leaves[leafindex].data.Value = nil
	} else {
		h.leaves[leafindex].data.Hashstr = data.Hashstr
		h.leaves[leafindex].data.Value = data.Value
	}
	pindex := h.getparentindex(h.leaves[leafindex].nodeindex)
	for {
		h.nodes[pindex].data.Hashstr = h.caculate(pindex)
		if pindex == 0 {
			break
		}
		pindex = h.getparentindex(pindex)
	}
	return nil
}
func (h *FixedHashtree) SetMultiLeaves(datas map[int]*LeafData) error {
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
			h.leaves[leafindex].data.Hashstr = nil
			h.leaves[leafindex].data.Value = nil
		} else {
			h.leaves[leafindex].data.Hashstr = leafdata.Hashstr
			h.leaves[leafindex].data.Value = leafdata.Value
		}
		pindexs[h.getparentindex(h.leaves[leafindex].nodeindex)] = nil
	}
	finish := false
	for {
		newpindex := make(map[int]*struct{}, 0)
		for pindex := range pindexs {
			h.nodes[pindex].data.Hashstr = h.caculate(pindex)
			if pindex == 0 {
				finish = true
				break
			}
			newpindex[h.getparentindex(pindex)] = nil
		}
		if finish {
			break
		}
		pindexs = newpindex
	}
	return nil
}
func (h *FixedHashtree) Export() map[int]*LeafData {
	result := make(map[int]*LeafData)
	for leafindex, leafdata := range h.leaves {
		result[leafindex] = &LeafData{
			Hashstr: leafdata.data.Hashstr,
			Value:   leafdata.data.Value,
		}
	}
	return result
}
func (h *FixedHashtree) GetSingleLeaf(leafindex int) (*LeafData, error) {
	if leafindex >= len(h.leaves) {
		return nil, ErrLeafIndexOutOfRange
	}
	return &LeafData{
		Hashstr: h.leaves[leafindex].data.Hashstr,
		Value:   h.leaves[leafindex].data.Value,
	}, nil
}
func (h *FixedHashtree) GetMultiLeaves(leafindexs []int) (map[int]*LeafData, error) {
	for _, leafindex := range leafindexs {
		if leafindex >= len(h.leaves) {
			return nil, ErrLeafIndexOutOfRange
		}
	}
	result := make(map[int]*LeafData)
	for _, leafindex := range leafindexs {
		result[leafindex] = &LeafData{
			Hashstr: h.leaves[leafindex].data.Hashstr,
			Value:   h.leaves[leafindex].data.Value,
		}
	}
	return result, nil
}
func (h *FixedHashtree) Different(other *FixedHashtree) ([]int, error) {
	if len(h.leaves) != len(other.leaves) {
		return nil, ErrLeafLength
	}
	result := make([]int, 0, 100)
	maydifindexs := make(map[int]*struct{})
	maydifindexs[0] = nil
	for {
		newmaydifindexs := make(map[int]*struct{})
		for nodeindex := range maydifindexs {
			if !bytes.Equal(h.nodes[nodeindex].data.Hashstr, other.nodes[nodeindex].data.Hashstr) {
				if childnodestart := h.getNodeStartIndexInChildPiece(nodeindex); childnodestart >= len(h.nodes) {
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
