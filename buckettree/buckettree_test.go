package buckettree

import (
	"bytes"
	"testing"
)

func Test_buckettree(t *testing.T) {
	t1 := New(6, 8)
	t1.UpdateSingle(1, []byte("0123456789"))
	newhashs := make(map[uint64][]byte)
	newhashs[2] = []byte("abcde")
	newhashs[3] = []byte("jklmn")
	t1.UpdateBatch(newhashs)
	datas := t1.Export()
	t2 := Rebuild(6, 8, datas)
	if t2 == nil {
		panic("rebuild failed")
	}
	if !bytes.Equal(t1.GetRootHash(), t2.GetRootHash()) {
		panic("different after rebuild")
	}
	datas[1] = []byte("aaa")
	datas[4] = []byte("xyz")
	if index, e := t1.SearchDifferent(datas); e != nil {
		panic(e)
	} else if len(index) != 2 {
		panic("different bucket num")
	} else if (index[0] == 1 && index[1] == 4) || (index[0] == 4 && index[1] == 1) {
	} else {
		panic("different bucket num index error")
	}
}
