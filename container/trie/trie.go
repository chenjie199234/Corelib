package trie

type Trie[T any] []*node[T]

type node[T any] struct {
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
				if child.str[0] == str[0] {
					child.Set(str, value)
					return
				}
			}
			n.children = append(n.children, &node[T]{
				children: make([]*node[T], 0, 3),
				str:      str,
				value:    value,
				exist:    true,
			})
			return
		}
		if str[i] != n.str[i] {
			newnode := &node[T]{
				children: make([]*node[T], 0, 3),
				str:      str[i:],
				value:    value,
				exist:    true,
			}
			oldnode := &node[T]{
				children: n.children,
				str:      n.str[i:],
				value:    n.value,
				exist:    n.exist,
			}
			n.children = make([]*node[T], 0, 3)
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
		children: n.children,
		str:      n.str[len(str):],
		value:    n.value,
		exist:    n.exist,
	}
	n.children = make([]*node[T], 0, 3)
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
				if child.str[0] == str[0] {
					return child.Get(str)
				}
			}
			return
		}
		if str[i] != n.str[i] {
			return
		}
	}
	return
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
		if child.str[0] == str[0] {
			child.Set(str, value)
			return
		}
	}
	*t = append(*t, &node[T]{
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
		if child.str[0] == str[0] {
			return child.Get(str)
		}
	}
	return
}
func (t *Trie[T]) Reset() {
	*t = make(Trie[T], 0, 3)
}
