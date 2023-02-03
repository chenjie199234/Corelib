package trie

// thread unsafe
type Trie[T any] []*node[T]

type node[T any] struct {
	parent   *node[T]
	children []*node[T]
	str      string
	value    T
	exist    bool
}

func (n *node[T]) Set(str string, value T) {
	if n.str == str {
		n.str = str
		n.value = value
		n.exist = true
		return
	}
	for i := range str {
		if i >= len(n.str) {
			str = str[i:]
			for _, child := range n.children {
				if child.str[0] != str[0] {
					continue
				}
				child.Set(str, value)
				return
			}
			n.children = append(n.children, &node[T]{
				parent:   n,
				children: make([]*node[T], 0, 3),
				str:      str,
				value:    value,
				exist:    true,
			})
			return
		}
		if str[i] != n.str[i] {
			newnode := &node[T]{
				parent:   n,
				children: make([]*node[T], 0, 3),
				str:      str[i:],
				value:    value,
				exist:    true,
			}
			oldnode := &node[T]{
				parent:   n,
				children: n.children,
				str:      n.str[i:],
				value:    n.value,
				exist:    n.exist,
			}
			n.children = make([]*node[T], 0, 2)
			n.children = append(n.children, oldnode)
			n.children = append(n.children, newnode)
			n.str = str[:i]
			var empty T
			n.value = empty
			n.exist = false
			return
		}
	}
	oldnode := &node[T]{
		parent:   n,
		children: n.children,
		str:      n.str[len(str):],
		value:    n.value,
		exist:    n.exist,
	}
	n.children = make([]*node[T], 0, 1)
	n.children = append(n.children, oldnode)
	n.str = str
	n.value = value
	n.exist = true
}
func (n *node[T]) Get(str string) (value T, ok bool) {
	if n.str == str {
		return n.value, n.exist
	}
	for i := range str {
		if i >= len(n.str) {
			str = str[i:]
			for _, child := range n.children {
				if child.str[0] != str[0] {
					continue
				}
				return child.Get(str)
			}
			return
		}
		if str[i] != n.str[i] {
			return
		}
	}
	return
}
func (n *node[T]) Del(str string) (ok bool) {
	if n.str == str {
		if !n.exist {
			return false
		}
		var empty T
		n.value = empty
		n.exist = false
		//compress
		self := n
		parent := self.parent
		for {
			if len(self.children) == 0 && parent != nil {
				for i, child := range parent.children {
					if child != self {
						continue
					}
					if i == len(parent.children)-1 {
						//delete the end element
						parent.children = parent.children[:len(parent.children)-1]
					} else {
						//swap to the end
						parent.children[i], parent.children[len(parent.children)-1] = parent.children[len(parent.children)-1], parent.children[i]
						//delete the end element
						parent.children = parent.children[:len(parent.children)-1]
					}
					break
				}
				self = parent
				parent = self.parent
				continue
			}
			if len(self.children) == 1 {
				child := self.children[0]
				self.children = child.children
				self.str += child.str
				self.value = child.value
				self.exist = child.exist
			}
			break
		}
		return true
	}
	for i := range str {
		if i >= len(n.str) {
			str = str[i:]
			for _, child := range n.children {
				if child.str[0] != str[0] {
					continue
				}
				return child.Del(str)
			}
			return false
		}
		if str[i] != n.str[i] {
			return false
		}
	}
	return false
}
func (n *node[T]) GetAll(prefix string, result map[string]T) {
	if result == nil {
		return
	}
	prefix += n.str
	if n.exist {
		result[prefix] = n.value
	}
	for _, child := range n.children {
		child.GetAll(prefix, result)
	}
}

func NewTrie[T any]() *Trie[T] {
	trie := make(Trie[T], 0, 3)
	return &trie
}
func (t *Trie[T]) Set(str string, value T) {
	if str == "" {
		return
	}
	for _, child := range *t {
		if child.str[0] != str[0] {
			continue
		}
		child.Set(str, value)
		return
	}
	*t = append(*t, &node[T]{
		parent:   nil,
		children: make([]*node[T], 0, 3),
		str:      str,
		value:    value,
		exist:    true,
	})
}
func (t *Trie[T]) Get(str string) (value T, ok bool) {
	if str == "" {
		return
	}
	for _, child := range *t {
		if child.str[0] != str[0] {
			continue
		}
		return child.Get(str)
	}
	return
}

// return true:del success,false:not exist
func (t *Trie[T]) Del(str string) bool {
	for i, child := range *t {
		if child.str[0] != str[0] {
			continue
		}
		if !child.Del(str) {
			return false
		}
		if child.exist || len(child.children) != 0 {
			return true
		}
		if i == len(*t)-1 {
			//delete the end element
			*t = (*t)[:len(*t)-1]
		} else {
			//swap to the end
			(*t)[i], (*t)[len(*t)-1] = (*t)[len(*t)-1], (*t)[i]
			//delete the end element
			*t = (*t)[:len(*t)-1]
		}
		return true
	}
	return false
}
func (t *Trie[T]) Reset() {
	*t = make(Trie[T], 0, 3)
}
func (t *Trie[T]) GetAll() map[string]T {
	result := make(map[string]T)
	for _, child := range *t {
		child.GetAll("", result)
	}
	return result
}
