package buckettree

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"hash"
)

type bucket struct {
	hashStr  []byte
	index    uint64
	parent   *bucket
	children []*bucket
}
type BucketTree struct {
	bucketNum uint64
	buckets   []*bucket
	treeTall  uint64
	treeWidth uint64
	root      *bucket
	encoder   hash.Hash
}

func pow(base, times uint64) uint64 {
	result := uint64(1)
	for times > 0 {
		result *= base
		times--
	}
	return result
}
func New(treeWidth, treeTall uint64) *BucketTree {
	return initialize(treeWidth, treeTall, nil)
}
func Rebuild(treeWidth, treeTall uint64, bucketHashs [][]byte) *BucketTree {
	return initialize(treeWidth, treeTall, bucketHashs)
}

func initialize(treeWidth, treeTall uint64, bucketHashs [][]byte) *BucketTree {
	if treeTall <= 0 || treeWidth <= 1 {
		return nil
	}
	num := pow(treeWidth, treeTall-1)
	isnew := false
	if bucketHashs == nil || len(bucketHashs) == 0 {
		//this is new
		isnew = true
	}
	if !isnew && uint64(len(bucketHashs)) != num {
		//this is rebuild,but the buckets num is different
		return nil
	}
	b := &BucketTree{
		bucketNum: num,
		treeTall:  treeTall,
		treeWidth: treeWidth,
		buckets:   make([]*bucket, num),
		encoder:   sha256.New(),
	}
	var tempStr []byte
	if isnew {
		tempStr = b.encoder.Sum(nil)
	}
	if b.treeTall == 1 {
		newBucket := &bucket{
			index:    0,
			parent:   nil,
			children: nil,
		}
		if isnew {
			newBucket.hashStr = tempStr
		} else {
			newBucket.hashStr = bucketHashs[0]
		}
		b.root = newBucket
		b.buckets[0] = newBucket
	} else {
		//first loop to get temp
		temp := make([]*bucket, b.bucketNum/b.treeWidth)
		for i := uint64(0); i < b.bucketNum; i += b.treeWidth {
			newParent := &bucket{
				hashStr:  nil,
				index:    i / b.treeWidth,
				parent:   nil,
				children: make([]*bucket, b.treeWidth),
			}
			s := []byte{}
			for j := uint64(0); j < b.treeWidth; j++ {
				newBucket := &bucket{
					index:    i + j,
					parent:   newParent,
					children: nil,
				}
				if isnew {
					newBucket.hashStr = tempStr
				} else {
					newBucket.hashStr = bucketHashs[i+j]
				}
				newParent.children[j] = newBucket
				b.buckets[i+j] = newBucket
				s = append(s, newBucket.hashStr...)
			}
			b.encoder.Write(s)
			newParent.hashStr = b.encoder.Sum(nil)
			b.encoder.Reset()
			temp[newParent.index] = newParent
		}
		for len(temp) > 1 {
			for i := uint64(0); i < uint64(len(temp)); i += b.treeWidth {
				newParent := &bucket{
					hashStr:  nil,
					index:    i / b.treeWidth,
					parent:   nil,
					children: make([]*bucket, b.treeWidth),
				}
				s := []byte{}
				for j := uint64(0); j < b.treeWidth; j++ {
					s = append(s, temp[i+j].hashStr...)
					temp[i+j].parent = newParent
					newParent.children[j] = temp[i+j]
				}
				b.encoder.Write(s)
				newParent.hashStr = b.encoder.Sum(nil)
				b.encoder.Reset()
				temp[newParent.index] = newParent
			}
			temp = temp[:uint64(len(temp))/b.treeWidth]
		}
		b.root = temp[0]
	}
	return b
}
func (b *BucketTree) UpdateAll() {
	num := b.bucketNum
	temp := make([]*bucket, b.bucketNum/b.treeWidth)
	for i := uint64(0); i < num; i += b.treeWidth {
		tempParent := b.buckets[i].parent
		s := []byte{}
		for _, v := range tempParent.children {
			s = append(s, v.hashStr...)
		}
		b.encoder.Write(s)
		tempParent.hashStr = b.encoder.Sum(nil)
		b.encoder.Reset()
		temp[i/b.treeWidth] = tempParent
	}
	for i := (b.treeTall - 1); i > 1; i-- {
		num = num / b.treeWidth
		for j := uint64(0); j < num; j += b.treeWidth {
			tempParent := temp[j].parent
			s := []byte{}
			for _, v := range tempParent.children {
				s = append(s, v.hashStr...)
			}
			b.encoder.Write(s)
			tempParent.hashStr = b.encoder.Sum(nil)
			b.encoder.Reset()
			temp[j/b.treeWidth] = tempParent
		}
		temp = temp[:num/b.treeWidth]
	}
}
func (b *BucketTree) UpdateBatch(newHashs map[uint64][]byte) {
	//split width treewidth
	split := make(map[uint64]struct{}, len(newHashs))
	count := 0
	for i, v := range newHashs {
		if i < b.bucketNum {
			split[i/b.treeWidth] = struct{}{}
			count++
			b.buckets[i].hashStr = v
		}
	}
	temp := make(map[uint64]*bucket, len(split))
	for i, _ := range split {
		tempParent := b.buckets[i*b.treeWidth].parent
		if tempParent != nil {
			s := []byte{}
			for _, v := range tempParent.children {
				s = append(s, v.hashStr...)
			}
			b.encoder.Write(s)
			tempParent.hashStr = b.encoder.Sum(nil)
			b.encoder.Reset()
			index := tempParent.index / b.treeWidth
			if tempParent.parent != nil {
				temp[index] = tempParent.parent
			}
		}
	}
	if len(temp) != 0 {
		b.updateParent(temp)
	}
}
func (b *BucketTree) updateParent(parents map[uint64]*bucket) {
	temp := make(map[uint64]*bucket, len(parents))
	for _, tempParent := range parents {
		s := []byte{}
		for _, v := range tempParent.children {
			s = append(s, v.hashStr...)
		}
		b.encoder.Write(s)
		tempParent.hashStr = b.encoder.Sum(nil)
		b.encoder.Reset()
		index := tempParent.index / b.treeWidth
		if tempParent.parent != nil {
			temp[index] = tempParent.parent
		}
	}
	if len(temp) != 0 {
		b.updateParent(temp)
	}
}
func (b *BucketTree) UpdateSingle(index uint64, newHash []byte) {
	if index < b.bucketNum {
		b.buckets[index].hashStr = newHash
		tempParent := b.buckets[index].parent
		for tempParent != nil {
			s := []byte{}
			for _, v := range tempParent.children {
				s = append(s, v.hashStr...)
			}
			b.encoder.Write(s)
			tempParent.hashStr = b.encoder.Sum(nil)
			b.encoder.Reset()
			tempParent = tempParent.parent
		}
	}
}
func (b *BucketTree) GetRootHash() []byte {
	return b.root.hashStr
}
func (b *BucketTree) GetBucketHash(i uint64) []byte {
	return b.buckets[i].hashStr
}
func (b *BucketTree) GetBucketNum() uint64 {
	return b.bucketNum
}
func (b *BucketTree) GetTreeTall() uint64 {
	return b.treeTall
}
func (b *BucketTree) GetTreeWidth() uint64 {
	return b.treeWidth
}
func (b *BucketTree) SearchDifferent(bucketHashs [][]byte) ([]uint64, error) {
	temptree := Rebuild(b.treeWidth, b.treeTall, bucketHashs)
	if temptree == nil {
		return nil, errors.New("different num of tree leaves!")
	}
	if bytes.Equal(b.root.hashStr, temptree.root.hashStr) {
		return nil, nil
	}
	index := make([]uint64, 0)
	compare(b.root, temptree.root, &index)
	return index, nil
}
func compare(local *bucket, other *bucket, index *[]uint64) {
	if len(local.children) == 0 && len(other.children) == 0 {
		*index = append(*index, local.index)
	} else {
		for i, _ := range local.children {
			if bytes.Compare(local.children[i].hashStr, other.children[i].hashStr) != 0 {
				compare(local.children[i], other.children[i], index)
			}
		}
	}
}
func (b *BucketTree) Export() [][]byte {
	temp := make([][]byte, b.bucketNum)
	for i := uint64(0); i < b.bucketNum; i++ {
		temp[i] = b.buckets[i].hashStr
	}
	return temp
}
