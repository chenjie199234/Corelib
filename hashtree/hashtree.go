//has data race
package hashtree

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"hash"
	"math"
	"unsafe"
)

var (
	ERROUTOFRANGE      = fmt.Errorf("index out of range")
	ERRDIFFERENTLENGTH = fmt.Errorf("length not equal")
)

type Hashtree struct {
	encoder hash.Hash
	nodes   []*node
	leaves  []*node
	width   int
	tall    int
}
type node struct {
	hashstr   []byte
	value     unsafe.Pointer
	nodeindex int
	leafindex int
}

func New(width, tall int) *Hashtree {
	if tall == 0 || width == 0 {
		return nil
	}
	nodesnum := 0
	for i := 1; i <= tall; i++ {
		nodesnum += int(math.Pow(float64(width), float64(i-1)))
	}
	leavesnum := int(math.Pow(float64(width), float64(tall-1)))
	instance := &Hashtree{
		encoder: md5.New(),
		nodes:   make([]*node, nodesnum),
		width:   width,
		tall:    tall,
	}
	emptyhash := instance.encoder.Sum(nil)
	instance.leaves = instance.nodes[len(instance.nodes)-leavesnum:]
	for i := range instance.nodes {
		instance.nodes[i] = &node{
			nodeindex: i,
		}
	}
	for i := range instance.leaves {
		instance.leaves[i].hashstr = emptyhash
		instance.leaves[i].leafindex = i
	}
	instance.UpdateAll()
	return instance
}
func (h *Hashtree) UpdateAll() {
	for i := len(h.nodes) - 1; i >= 0; i -= h.width {
		pindex := h.getparentindex(i)
		h.nodes[pindex].hashstr = h.caculate(pindex)
	}
}
func (h *Hashtree) Reset() {
	h.encoder.Reset()
	emptyhash := h.encoder.Sum(nil)
	for i := range h.leaves {
		h.leaves[i].hashstr = emptyhash
		h.leaves[i].value = nil
	}
	h.UpdateAll()
}
func (h *Hashtree) Rebuild(data []*LeafData) error {
	if len(data) != len(h.leaves) {
		return ERRDIFFERENTLENGTH
	}
	for i := range h.leaves {
		h.leaves[i].hashstr = data[i].Hashstr
		h.leaves[i].value = data[i].Value
	}
	h.UpdateAll()
	return nil
}
func (h *Hashtree) GetLeavesNum() int {
	return len(h.leaves)
}
func (h *Hashtree) GetRootHash() []byte {
	return h.nodes[0].hashstr
}

type LeafData struct {
	Hashstr []byte
	Value   unsafe.Pointer
}

func (h *Hashtree) SetSingleLeaf(index int, data *LeafData) error {
	if index >= len(h.leaves) {
		return ERROUTOFRANGE
	}
	h.leaves[index].hashstr = data.Hashstr
	h.leaves[index].value = data.Value
	pindex := h.getparentindex(h.leaves[index].nodeindex)
	for {
		h.nodes[pindex].hashstr = h.caculate(pindex)
		if pindex == 0 {
			break
		}
		pindex = h.getparentindex(pindex)
	}
	return nil
}
func (h *Hashtree) SetMultiLeaves(datas map[int]*LeafData) error {
	for index := range datas {
		if index >= len(h.leaves) {
			return ERROUTOFRANGE
		}
	}
	pindexs := make(map[int]struct{}, 10)
	for index, data := range datas {
		h.leaves[index].hashstr = data.Hashstr
		h.leaves[index].value = data.Value
		pindexs[h.getparentindex(h.leaves[index].nodeindex)] = struct{}{}
	}
	finish := false
	for {
		newpindex := make(map[int]struct{}, 0)
		for pindex := range pindexs {
			h.nodes[pindex].hashstr = h.caculate(pindex)
			if pindex == 0 {
				finish = true
				break
			}
			newpindex[h.getparentindex(pindex)] = struct{}{}
		}
		if finish {
			break
		}
		pindexs = newpindex
	}
	return nil
}
func (h *Hashtree) caculate(pindex int) []byte {
	piece := make([][]byte, h.width)
	for j := 0; j < h.width; j++ {
		piece[j] = h.nodes[pindex*h.width+1+j].hashstr
	}
	h.encoder.Reset()
	h.encoder.Write(bytes.Join(piece, nil))
	return h.encoder.Sum(nil)
}
func (h *Hashtree) GetLeaf(index int) (unsafe.Pointer, error) {
	if index >= len(h.leaves) {
		return nil, ERROUTOFRANGE
	}
	return h.leaves[index].value, nil
}
func (h *Hashtree) getStartIndexInPiece(index int) int {
	return (((index + h.width - 1) / h.width) * h.width)
}
func (h *Hashtree) getStartIndexNextPiece(index int) int {
	return index*h.width + 1
}
func (h *Hashtree) getparentindex(index int) int {
	sindex := h.getStartIndexInPiece(index)
	return (sindex - 1) / h.width
}
func (h *Hashtree) Different(other *Hashtree) (map[int]*LeafData, error) {
	if len(h.leaves) != len(other.leaves) {
		return nil, ERRDIFFERENTLENGTH
	}
	result := make(map[int]*LeafData)
	maydifindexs := make(map[int]struct{})
	maydifindexs[0] = struct{}{}
	for {
		newmaydifindexs := make(map[int]struct{})
		for index := range maydifindexs {
			if !bytes.Equal(h.nodes[index].hashstr, other.nodes[index].hashstr) {
				if nextstart := h.getStartIndexNextPiece(index); nextstart >= len(h.nodes) {
					result[h.nodes[index].leafindex] = &LeafData{
						Hashstr: h.nodes[index].hashstr,
						Value:   h.nodes[index].value,
					}
				} else {
					for i := 0; i < 10; i++ {
						newmaydifindexs[nextstart+i] = struct{}{}
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
