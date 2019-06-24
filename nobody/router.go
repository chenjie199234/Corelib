package nobody

import (
	"net/http"
)

type node struct {
	path        string
	children    []*node
	specialNode *node
	parent      *node
	handler     []HandleFunc //handle function
}

type Router struct {
	root   string
	head   *node
	get    *node
	post   *node
	delete *node
	put    *node
}

func (r *Router) Head(path string, handler ...HandleFunc) {
	original := path
	if path == "" {
		return
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		r.head.insert(original, formatted, temp)
		r.head.rebuild()
	}
}
func (r *Router) Get(path string, handler ...HandleFunc) {
	original := path
	if path == "" {
		return
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		r.get.insert(original, formatted, temp)
		r.get.rebuild()
	}
}
func (r *Router) Post(path string, handler ...HandleFunc) {
	original := path
	if path == "" {
		return
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		r.post.insert(original, formatted, temp)
		r.post.rebuild()
	}
}
func (r *Router) Delete(path string, handler ...HandleFunc) {
	original := path
	if path == "" {
		return
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		r.delete.insert(original, formatted, temp)
		r.delete.rebuild()
	}
}
func (r *Router) Put(path string, handler ...HandleFunc) {
	original := path
	if path == "" {
		return
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	temp := make([]HandleFunc, 0)
	for _, v := range handler {
		if v != nil {
			temp = append(temp, v)
		}
	}
	if len(temp) != 0 {
		r.put.insert(original, formatted, temp)
		r.put.rebuild()
	}
}

func (r *Router) Group(path string) *Router {
	for _, v := range path {
		if v == '*' {
			panic("can't use '*' as group path\n")
		}
	}
	original := path
	if path == "" {
		return nil
	} else {
		path = formatPath(r.root + path)
	}
	formatted := path
	r.get.insert(original, formatted, nil)
	r.post.insert(original, formatted, nil)
	r.delete.insert(original, formatted, nil)
	r.put.insert(original, formatted, nil)
	r.head.insert(original, formatted, nil)
	return &Router{
		root:   path,
		get:    r.get,
		post:   r.post,
		delete: r.delete,
		put:    r.put,
		head:   r.head,
	}
}
func (r *Router) init() {
	r.root = "/"
	r.get = &node{
		parent:      nil,
		path:        "/",
		children:    make([]*node, 0),
		specialNode: nil,
		handler:     nil,
	}
	r.post = &node{
		parent:      nil,
		path:        "/",
		children:    make([]*node, 0),
		specialNode: nil,
		handler:     nil,
	}
	r.delete = &node{
		parent:      nil,
		path:        "/",
		children:    make([]*node, 0),
		specialNode: nil,
		handler:     nil,
	}
	r.put = &node{
		parent:      nil,
		path:        "/",
		children:    make([]*node, 0),
		specialNode: nil,
		handler:     nil,
	}
	r.head = &node{
		parent:      nil,
		path:        "/",
		children:    make([]*node, 0),
		specialNode: nil,
		handler:     nil,
	}
}
func (r *Router) search(ctx *Context) {
	switch ctx.GetMethod() {
	case http.MethodGet:
		r.get.search(ctx)
	case http.MethodPost:
		r.post.search(ctx)
	case http.MethodPut:
		r.put.search(ctx)
	case http.MethodDelete:
		r.delete.search(ctx)
	case http.MethodHead:
		r.head.search(ctx)
	default:
	}
	return
}
func (n *node) insert(original, formatted string, handler []HandleFunc) {
	path := formatted
	if !checkPath(formatted) {
		panic("syntax error:\norigin url " + original + "\nformatted url " + formatted + "\n")
	}
	for len(path) > 0 {
		if path == n.path {
			if n.handler == nil || len(n.handler) == 0 {
				n.handler = handler
				return
			} else if handler == nil || len(handler) == 0 {
				//do nothing
				return
			} else {
				panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n.specialNode) + "\n")
			}
		}
		a := len(n.path)
		b := len(path)
		i := 0
		for ; i < min(a, b); i++ {
			if n.path[i] != path[i] {
				break
			}
		}
		if i < a {
			//split
			newNode := &node{
				parent:      nil,
				path:        n.path[i:],
				children:    n.children,
				specialNode: n.specialNode,
				handler:     n.handler,
			}
			n.path = n.path[:i]
			n.children = []*node{newNode}
			n.handler = nil
			n.specialNode = nil
		}
		if i < b {
			find := false
			for _, child := range n.children {
				if child.path[0] == path[i] {
					n = child
					find = true
					break
				}
			}
			if find {
				path = path[i:]
				continue
			}
			switch path[i] {
			case ':':
				if len(n.children) != 0 {
					panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n) + "\n")
				}
				path = path[i:]
				temp := ""
				for ii, v := range path {
					if v == '/' {
						temp = path[:ii+1]
						break
					}
				}
				if n.specialNode != nil {
					if n.specialNode.path != temp {
						panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n.specialNode) + "\n")
					} else {
						n = n.specialNode
						continue
					}
				} else {
					//split
					newNode := &node{
						parent:      nil,
						path:        temp,
						children:    make([]*node, 0),
						specialNode: nil,
						handler:     nil,
					}
					n.specialNode = newNode
					n = newNode
					continue
				}
			case '*':
				if len(n.children) != 0 {
					panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n) + "\n")
				} else if n.specialNode != nil {
					panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n.specialNode) + "\n")
				} else {
					//split
					newNode := &node{
						parent:      nil,
						path:        path[i:],
						children:    nil,
						specialNode: nil,
						handler:     handler,
					}
					n.specialNode = newNode
					return
				}
			default:
				if n.specialNode != nil {
					panic("path conflict:\noriginal url " + original + "\nformatted url " + formatted + "\nconflict url " + organizePath(n.specialNode) + "\n")
				}
				path = path[i:]
				temp := ""
				for ii, v := range path {
					if v == '/' && ii != len(path)-1 && (path[ii+1] == ':' || path[ii+1] == '*') {
						temp = path[:ii+1]
						break
					}
				}
				if temp == "" {
					temp = path
				}
				//split
				newNode := &node{
					parent:      nil,
					path:        temp,
					children:    make([]*node, 0),
					specialNode: nil,
					handler:     nil,
				}
				n.children = append(n.children, newNode)
				n = newNode
			}
		}
	}
}
func (n *node) search(ctx *Context) {
	path := ctx.GetPath()
	//prepare the start node
	if path[0] != '/' {
		for _, v := range n.children {
			if v.path[0] == path[0] {
				n = v
				break
			}
		}
		if n.path[0] == '/' && n.specialNode != nil {
			n = n.specialNode
		}
		if n.path[0] == '/' {
			//didn't match
			return
		}
	}
walk:
	switch n.path[0] {
	case ':':
		if ctx.params == nil {
			ctx.params = make(map[string]string, 2)
		}
		i := 0
		for ; i < len(path); i++ {
			if path[i] == '/' {
				break
			}
		}

		if i == len(path)-1 {
			//find,path is end of '/'
			ctx.params[n.path[1:len(n.path)-1]] = path[:i]
			ctx.calls = n.handler
			break
		}
		if i == len(path) {
			//find,path is not end of '/'
			ctx.params[n.path[1:len(n.path)-1]] = path
			ctx.calls = n.handler
			break
		}
		ctx.params[n.path[1:len(n.path)-1]] = path[:i]
		path = path[i+1:]
		for _, child := range n.children {
			if child.path[0] == path[0] {
				n = child
				goto walk
			}
		}
		if n.specialNode != nil {
			n = n.specialNode
			goto walk
		}
		//didn't match
		break
	case '*':
		if ctx.params == nil {
			ctx.params = make(map[string]string, 1)
		}
		ctx.params["*"] = path
		ctx.calls = n.handler
		break
	default:
		if path == n.path {
			//find
			ctx.calls = n.handler
			break
		} else if len(path) > len(n.path) {
			if path[:len(n.path)] == n.path {
				path = path[len(n.path):]
				for _, child := range n.children {
					if child.path[0] == path[0] {
						n = child
						goto walk
					}
				}
				if n.specialNode != nil {
					n = n.specialNode
					goto walk
				}
				//didn't match
				break
			} else {
				//didn't match
				break
			}
		} else if len(path)+1 == len(n.path) && n.path[len(n.path)-1] == '/' && path == n.path[:len(n.path)-1] {
			//n.path-->abc/
			//path---->abc
			//find
			ctx.calls = n.handler
			break
		} else {
			//didn't match
			break
		}
	}
}
func (p *node) rebuild() {
	for _, v := range p.children {
		v.parent = p
		v.rebuild()
	}
	if p.specialNode != nil {
		p.specialNode.parent = p
		p.specialNode.rebuild()
	}
}
func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}
func formatPath(path string) string {
	if path == "" {
		return path
	}
	n := len(path)
	//lazy mem copy
	buf := []byte(nil)
	cursorPos := 0
	for i := 0; i < n; {
		switch {
		case path[i] == '/':
			//compress series '/'
			i++
			series := false
			for i < n && path[i] == '/' {
				series = true
				i++
			}
			if buf != nil {
				buf[cursorPos] = '/'
			} else if series && i != n {
				buf = make([]byte, n)
				copy(buf, path[:cursorPos+1])
			}
			cursorPos++
		case path[i] == '.' && i+1 == n:
			i++
		case path[i] == '.' && path[i+1] == '/':
			if buf == nil {
				buf = make([]byte, n)
				copy(buf, path[:cursorPos])
			}
			if cursorPos > 0 && buf[cursorPos-1] == '/' {
				i += 2
			} else {
				i++
			}
		case path[i] == '.' && path[i+1] == '.':
			//compress series '.'
			i += 2
			for i < n && path[i] == '.' {
				i++
			}
			switch {
			case i == n:
				break
			case path[i] == '/':
				if buf == nil {
					buf = make([]byte, n)
					copy(buf, path[:cursorPos])
				}
				if cursorPos > 0 && buf[cursorPos-1] == '/' {
					cursorPos--
					for cursorPos > 0 {
						cursorPos--
						if buf[cursorPos] == '/' {
							break
						}
					}
					if cursorPos == 0 {
						buf[0] = '/'
						cursorPos = 1
					} else {
						cursorPos++
					}
					//compress series '/'
					i++
					for i < n && path[i] == '/' {
						i++
					}
				}
			default:
				if buf == nil {
					buf = make([]byte, n)
					copy(buf, path[:cursorPos])
				}
				buf[cursorPos] = '.'
				cursorPos++
			}
		default:
			if buf != nil {
				buf[cursorPos] = path[i]
			}
			cursorPos++
			i++
		}
	}
	if cursorPos == 0 {
		return "/"
	}
	if buf == nil {
		if path[cursorPos-1] == '/' {
			if path[0] == '/' {
				return path[:cursorPos]
			} else {
				return "/" + path[:cursorPos]
			}
		} else {
			if path[0] == '/' {
				return path[:cursorPos] + "/"
			} else {
				return "/" + path[:cursorPos] + "/"
			}
		}
	} else {
		if buf[cursorPos-1] == '/' {
			if buf[0] == '/' {
				return string(buf[:cursorPos])
			} else {
				return "/" + string(buf[:cursorPos])
			}
		} else {
			if buf[0] == '/' {
				return string(buf[:cursorPos]) + "/"
			} else {
				return "/" + string(buf[:cursorPos]) + "/"
			}
		}
	}
}
func checkPath(path string) bool {
	if path == "" {
		return false
	}
	if path[0] != '/' {
		return false
	}
	exist := make(map[string]struct{}, 0)
	n := len(path)
	for i := 0; i < n; {
		if path[i] == ':' {
			if path[i-1] != '/' || i == n-1 || path[i+1] == '/' || path[i+1] == '*' {
				return false
			} else {
				temp := ""
				index := i + 1
				for ; index < len(path); index++ {
					if path[index] == '/' {
						temp = path[i+1 : index]
						break
					}
				}
				if temp == "" {
					temp = path[i+1:]
				}
				if _, ok := exist[temp]; ok {
					return false
				} else {
					exist[temp] = struct{}{}
				}
				i = index + 1
			}
		} else if path[i] == '*' && (path[i-1] != '/' || i != n-2 || path[i+1] != '/') {
			return false
		} else {
			i++
		}
	}
	return true
}
func organizePath(n *node) string {
	for {
		if len(n.children) != 0 {
			n = n.children[0]
		} else if n.specialNode != nil {
			n = n.specialNode
		} else {
			break
		}
	}
	result := ""
	for n.parent != nil {
		result = n.path + result
		n = n.parent
	}
	return "/" + result
}
