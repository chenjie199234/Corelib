package merkletree

import (
	"bytes"
	"testing"
)

func Test_merkletree(t *testing.T) {
	tree1 := New()
	data := make([][]byte, 5)
	data[0] = []byte("0")
	data[1] = []byte("1")
	data[2] = []byte("2")
	data[3] = []byte("3")
	data[4] = []byte("4")
	for _, d := range data {
		tree1.Push(d)
	}
	e := tree1.Export()
	tree2 := Rebuild(e)
	for i := 0; i < len(data); i++ {
		if !bytes.Equal(tree1.Export()[i], tree2.Export()[i]) {
			panic("export not equal")
		}
	}
	if !bytes.Equal(tree1.root.hashStr, tree2.root.hashStr) {
		panic("root hash not equal")
	}
	tree1.Push([]byte("5"))
	tree2.Push([]byte("5"))
	if !bytes.Equal(tree1.root.hashStr, tree2.root.hashStr) {
		panic("root hash not equal after push new hash")
	}
	route := tree1.GetVerifyRoute([]byte("0"))
	if !VerifyRoute(tree1.root.hashStr, []byte("0"), route) {
		panic("verify failed")
	}
}
