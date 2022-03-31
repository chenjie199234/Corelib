package hashtree

import (
	"bytes"
	"crypto/md5"
	"testing"
)

type data struct {
}

func Test_Hashtree(t *testing.T) {
	test_fixedhashtree(t)
	test_flexiblehashtree(t)
}
func test_fixedhashtree(t *testing.T) {
	tree1 := NewFixedHashtree[*data](md5.New(), 3, 3)
	tree1.SetSingle(0, &LeafData[*data]{
		Hstr: []byte("0"),
	})
	tree1.SetSingle(1, &LeafData[*data]{
		Hstr: []byte("1"),
	})
	tree1.SetSingle(2, &LeafData[*data]{
		Hstr: []byte("2"),
	})
	tree1.SetSingle(3, &LeafData[*data]{
		Hstr: []byte("3"),
	})
	tree1.SetSingle(4, &LeafData[*data]{
		Hstr: []byte("4"),
	})
	tree1.SetSingle(5, &LeafData[*data]{
		Hstr: []byte("5"),
	})
	tree1.SetSingle(6, &LeafData[*data]{
		Hstr: []byte("6"),
	})
	tree1.SetSingle(7, &LeafData[*data]{
		Hstr: []byte("7"),
	})
	tree1.SetSingle(8, &LeafData[*data]{
		Hstr: []byte("8"),
	})
	tree1.SetSingle(9, &LeafData[*data]{
		Hstr: []byte("9"),
	})
	tree1.SetSingle(10, &LeafData[*data]{
		Hstr: []byte("10"),
	})
	tree2 := NewFixedHashtree[*data](md5.New(), 3, 3)
	tree2.SetMulti(map[int]*LeafData[*data]{
		0:  {Hstr: []byte("0")},
		1:  {Hstr: []byte("1")},
		2:  {Hstr: []byte("2")},
		3:  {Hstr: []byte("3")},
		4:  {Hstr: []byte("4")},
		5:  {Hstr: []byte("5")},
		6:  {Hstr: []byte("6")},
		7:  {Hstr: []byte("7")},
		8:  {Hstr: []byte("8")},
		9:  {Hstr: []byte("9")},
		10: {Hstr: []byte("10")},
	})
	r1 := tree1.GetRootHash()
	r2 := tree2.GetRootHash()
	if !bytes.Equal(r1, r2) {
		t.Fatal("set single and set multi make different result")
	}
	tree2.Rebuild(tree1.Export())
	r2 = tree2.GetRootHash()
	if !bytes.Equal(r1, r2) {
		t.Fatal("rebuild make different result")
	}
	tree2.SetMulti(map[int]*LeafData[*data]{
		10: {Hstr: nil},
		3:  {Hstr: []byte("a")},
		12: {Hstr: []byte("b")},
	})
	d, _ := tree1.Different(tree2)
	if len(d) != 3 {
		t.Fatal("different failed")
	}
	for i, v := range d {
		if (i == 0 && v != 3) || (i == 1 && v != 10) || (i == 2 && v != 12) {
			t.Fatal("different failed")
		}
	}
}
func test_flexiblehashtree(t *testing.T) {
	tree1 := NewFlexibleHashtree[*data](md5.New(), 3)
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("0"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("1"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("2"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("3"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("4"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("5"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("6"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("7"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("8"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("9"),
	})
	tree1.PushSingle(&LeafData[*data]{
		Hstr: []byte("10"),
	})
	tree2 := NewFlexibleHashtree[*data](md5.New(), 3)
	tree2.PushMulti([]*LeafData[*data]{
		{Hstr: []byte("0")},
		{Hstr: []byte("1")},
		{Hstr: []byte("2")},
		{Hstr: []byte("3")},
		{Hstr: []byte("4")},
		{Hstr: []byte("5")},
		{Hstr: []byte("6")},
		{Hstr: []byte("7")},
		{Hstr: []byte("8")},
		{Hstr: []byte("9")},
		{Hstr: []byte("10")},
	})
	r1 := tree1.GetRootHash()
	r2 := tree2.GetRootHash()
	if !bytes.Equal(r1, r2) {
		t.Fatal("set single and set multi make different result")
	}
	tree2.Rebuild(tree1.Export())
	r2 = tree2.GetRootHash()
	if !bytes.Equal(r1, r2) {
		t.Fatal("rebuild make different result")
	}
	tree2.Rebuild([]*LeafData[*data]{
		{Hstr: []byte("0")},
		{Hstr: []byte("1")},
		{Hstr: []byte("a")},
		{Hstr: []byte("3")},
		{Hstr: []byte("4")},
		{Hstr: []byte("5")},
		{Hstr: []byte("6")},
		{Hstr: []byte("b")},
		{Hstr: []byte("8")},
		{Hstr: []byte("9")},
		{Hstr: []byte("10")},
		{Hstr: []byte("11")},
	})
	d := tree1.Different(tree2)
	if len(d) != 3 {
		t.Fatal("different failed")
	}
	for i, v := range d {
		if (i == 0 && v != 2) || (i == 1 && v != 7) || (i == 2 && v != 11) {
			t.Fatal("different failed")
		}
	}
}
