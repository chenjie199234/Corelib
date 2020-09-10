package hashtree

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"testing"
)

func Test_Hashtree(t *testing.T) {
	htree := New(10, 3)
	if htree.GetLeavesNum() != 100 {
		panic(fmt.Sprintf("leave num error:%d", htree.GetLeavesNum()))
	}
	encoder := md5.New()
	emptyhash := encoder.Sum(nil)
	for i := 0; i < 100; i++ {
		if !bytes.Equal(emptyhash, htree.leaves[i].hashstr) {
			panic("level 3 hash not equal")
		}
	}
	encoder.Write(bytes.Repeat(emptyhash, 10))
	level2hash := encoder.Sum(nil)
	for i := 1; i <= 10; i++ {
		if !bytes.Equal(htree.nodes[i].hashstr, level2hash) {
			panic("level 2 hash not equal")
		}
	}
	encoder.Reset()
	encoder.Write(bytes.Repeat(level2hash, 10))
	level1hash := encoder.Sum(nil)
	if !bytes.Equal(htree.nodes[0].hashstr, level1hash) {
		panic("level 1 hash not equal")
	}
	htree.SetSingleLeaf(0, &LeafData{
		Hashstr: []byte("123"),
	})
	encoder.Reset()
	encoder.Write(append([]byte("123"), bytes.Repeat(emptyhash, 9)...))
	newlevel2hash := encoder.Sum(nil)
	if !bytes.Equal(htree.nodes[1].hashstr, newlevel2hash) {
		panic("new level 2 hash not equal")
	}
	encoder.Reset()
	encoder.Write(append(newlevel2hash, bytes.Repeat(level2hash, 9)...))
	newlevel1hash := encoder.Sum(nil)
	if !bytes.Equal(htree.nodes[0].hashstr, newlevel1hash) {
		panic("new level 1 hash not equal")
	}
	htree.Reset()
	if !bytes.Equal(htree.nodes[0].hashstr, level1hash) {
		panic("reset hash not equal")
	}
	datas := make(map[int]*LeafData)
	datas[0] = &LeafData{
		Hashstr: []byte("123"),
	}
	datas[10] = &LeafData{
		Hashstr: []byte("123"),
	}
	htree.SetMultiLeaves(datas)
	encoder.Reset()
	encoder.Write(append(append(newlevel2hash, newlevel2hash...), bytes.Repeat(level2hash, 8)...))
	newlevel1hash = encoder.Sum(nil)
	if !bytes.Equal(newlevel1hash, htree.nodes[0].hashstr) {
		panic("multi new level 1 hash not equal")
	}
	differenthtree := New(10, 3)
	data, e := htree.Different(differenthtree)
	if e != nil {
		panic("search different error:" + e.Error())
	}
	if _, ok := data[0]; !ok {
		panic("different index error")
	}
	if _, ok := data[10]; !ok {
		panic("different index error")
	}
}
