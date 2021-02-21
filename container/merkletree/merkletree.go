package merkletree

import (
	"bytes"
	"crypto/sha256"
	"hash"

	"github.com/chenjie199234/Corelib/util/common"
)

type treeNode struct {
	hashStr []byte
	parent  *treeNode
	left    *treeNode
	right   *treeNode
}

type listNode struct {
	data *treeNode
	next *listNode
	prev *listNode
}

type list struct {
	head   *listNode
	curpos *listNode
	length uint64
}

func (l *list) insert(data *treeNode) {
	newNode := &listNode{
		data: data,
		next: nil,
		prev: nil,
	}
	l.length++
	if l.head == nil {
		l.head = newNode
		l.curpos = newNode
		return
	}
	if l.curpos.next != nil {
		newNode.next = l.curpos.next
		newNode.prev = l.curpos
		l.curpos.next.prev = newNode
		l.curpos.next = newNode
		//move the curpos
		l.curpos = l.curpos.next.next
	} else {
		//curpos is the end of the current list
		newNode.prev = l.curpos
		l.curpos.next = newNode
		//curpos reset to the head of the current list
		l.curpos = l.head
	}
}
func (l *list) push(data *treeNode) {
	newNode := &listNode{
		data: data,
		next: nil,
		prev: nil,
	}
	l.length++
	if l.head == nil {
		l.head = newNode
		l.curpos = newNode
		return
	}
	newNode.prev = l.curpos
	l.curpos.next = newNode
	l.curpos = newNode
}
func (l *list) getcur() *listNode {
	return l.curpos
}
func (l *list) setcur(pos int) {
	l.curpos = l.head
	pos--
	for pos >= 0 {
		pos--
		l.curpos = l.curpos.next
	}
}
func (l *list) gethead() *listNode {
	return l.head
}
func (l *list) getlen() uint64 {
	return l.length
}

type MerkleTree struct {
	root    *treeNode
	encoder hash.Hash
	list    *list
	nodes   map[string]*treeNode
}

func New() *MerkleTree {
	return &MerkleTree{
		root:    nil,
		encoder: sha256.New(),
		list:    new(list),
		nodes:   make(map[string]*treeNode),
	}
}
func Rebuild(hashs [][]byte) *MerkleTree {
	t := &MerkleTree{
		root:    nil,
		encoder: sha256.New(),
		list:    new(list),
		nodes:   make(map[string]*treeNode),
	}
	if len(hashs) == 0 {
		return t
	} else if len(hashs) == 1 {
		newNode := &treeNode{
			hashStr: hashs[0],
			parent:  nil,
			left:    nil,
			right:   nil,
		}
		t.list.insert(newNode)
		t.nodes[common.Byte2str(hashs[0])] = newNode
		t.root = newNode
		return t
	}
	treeTall := uint(0)
	for (1 << treeTall) < len(hashs) {
		treeTall++
	}
	temp := make([]*treeNode, 1<<(treeTall-1))
	temppos := 0
	redundancy := len(hashs) - (1 << (treeTall - 1))
	for i := 0; i < len(hashs); {
		if redundancy > 0 {
			newParent := &treeNode{}
			newLeft := &treeNode{
				hashStr: hashs[i],
				left:    nil,
				right:   nil,
				parent:  newParent,
			}
			newRight := &treeNode{
				hashStr: hashs[i+1],
				left:    nil,
				right:   nil,
				parent:  newParent,
			}
			newParent.left = newLeft
			newParent.right = newRight
			t.encoder.Write(append(newLeft.hashStr, newRight.hashStr...))
			newParent.hashStr = t.encoder.Sum(nil)
			t.encoder.Reset()
			t.list.push(newLeft)
			t.list.push(newRight)
			t.nodes[common.Byte2str(newLeft.hashStr)] = newLeft
			t.nodes[common.Byte2str(newRight.hashStr)] = newRight
			temp[temppos] = newParent
			i += 2
			redundancy--
		} else {
			newNode := &treeNode{
				hashStr: hashs[i],
				parent:  nil,
				left:    nil,
				right:   nil,
			}
			t.list.push(newNode)
			t.nodes[common.Byte2str(newNode.hashStr)] = newNode
			temp[temppos] = newNode
			i++
		}
		temppos++
	}
	for len(temp) > 1 {
		temppos = 0
		for i := 0; i < len(temp); i += 2 {
			newParent := &treeNode{
				hashStr: nil,
				parent:  nil,
				left:    temp[i],
				right:   temp[i+1],
			}
			newParent.left.parent = newParent
			newParent.right.parent = newParent
			t.encoder.Write(append(newParent.left.hashStr, newParent.right.hashStr...))
			newParent.hashStr = t.encoder.Sum(nil)
			t.encoder.Reset()
			temp[temppos] = newParent
			temppos++
		}
		temp = temp[:temppos]
	}
	t.root = temp[0]
	t.list.setcur(2 * (len(hashs) - (1 << (treeTall - 1))))
	return t
}
func (t *MerkleTree) Push(newHash []byte) {
	newNode := &treeNode{
		hashStr: newHash,
		parent:  nil,
		left:    nil,
		right:   nil,
	}
	t.nodes[common.Byte2str(newHash)] = newNode
	if t.root == nil {
		t.root = newNode
		t.list.insert(newNode)
		return
	}
	oldNode := t.list.getcur().data
	newParnet := &treeNode{
		hashStr: nil,
		parent:  oldNode.parent,
		left:    oldNode,
		right:   newNode,
	}
	if oldNode.parent != nil {
		if oldNode.parent.left == oldNode {
			oldNode.parent.left = newParnet
		} else {
			oldNode.parent.right = newParnet
		}
	} else {
		t.root = newParnet
	}
	newNode.parent = newParnet
	oldNode.parent = newParnet
	for newParnet != nil {
		t.encoder.Write(append(newParnet.left.hashStr, newParnet.right.hashStr...))
		newParnet.hashStr = t.encoder.Sum(nil)
		t.encoder.Reset()
		newParnet = newParnet.parent
	}
	t.list.insert(newNode)
}
func (t *MerkleTree) Export() [][]byte {
	if t.list.getlen() == 0 {
		return nil
	}
	result := make([][]byte, t.list.getlen())
	cur := t.list.gethead()
	i := 0
	for cur != nil {
		result[i] = cur.data.hashStr
		cur = cur.next
		i++
	}
	return result
}
func (t *MerkleTree) GetRootHash() []byte {
	if t.root == nil {
		return nil
	}
	return t.root.hashStr
}
func VerifyRoute(root, origin []byte, route [][]byte) bool {
	if len(route) == 0 {
		return false
	}
	encoder := sha256.New()
	for _, v := range route {
		if v[0] == 'r' {
			encoder.Write(append(origin, v[1:]...))
			origin = encoder.Sum(nil)
			encoder.Reset()
		} else if v[0] == 'l' {
			encoder.Write(append(v[1:], origin...))
			origin = encoder.Sum(nil)
			encoder.Reset()
		} else {
			return false
		}
	}
	return bytes.Equal(root, origin)
}
func (t *MerkleTree) GetVerifyRoute(verifyHash []byte) [][]byte {
	node, ok := t.nodes[common.Byte2str(verifyHash)]
	if !ok {
		return nil
	}
	result := make([][]byte, 0)
	scan(node, &result)
	return result
}
func scan(node *treeNode, result *[][]byte) {
	if node.parent == nil {
		return
	}
	if node.parent.left == node {
		*result = append(*result, append([]byte{'r'}, node.parent.right.hashStr...))
		scan(node.parent, result)
	} else {
		*result = append(*result, append([]byte{'l'}, node.parent.left.hashStr...))
		scan(node.parent, result)
	}
}
