package web

import (
	"net/http"
	"strings"

	"github.com/chenjie199234/Corelib/container/trie"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
)

type router struct {
	getTree         *trie.Trie[http.HandlerFunc]
	postTree        *trie.Trie[http.HandlerFunc]
	putTree         *trie.Trie[http.HandlerFunc]
	patchTree       *trie.Trie[http.HandlerFunc]
	deleteTree      *trie.Trie[http.HandlerFunc]
	rewriteHandler  func(originurl, method string) (newurl string, ok bool)
	notFoundHandler http.HandlerFunc
	optionsHandler  http.HandlerFunc
	srcHandler      http.HandlerFunc
}

func newRouter(srcroot string) *router {
	r := &router{
		getTree:    trie.NewTrie[http.HandlerFunc](),
		postTree:   trie.NewTrie[http.HandlerFunc](),
		putTree:    trie.NewTrie[http.HandlerFunc](),
		patchTree:  trie.NewTrie[http.HandlerFunc](),
		deleteTree: trie.NewTrie[http.HandlerFunc](),
	}
	if srcroot != "" {
		r.srcHandler = http.StripPrefix("/src", http.FileServer(http.Dir(srcroot))).ServeHTTP
	}
	return r
}

// the first character must be slash(/)
//      api/abc -> /api/abc
// remove tail slash(/)
//      /api/abc/ -> /api/abc
// multi series slash(///) -> single slash(/)
//      /api//abc -> /api/abc
// . -> current dir
//      /api/abc/. -> /api/abc                   (match)
//      /api/./abc -> /api/abc                   (match)
//      /api./abc  -> /api./abc                  (not match)
//      /api/.abc  -> /api/.abc                  (not match)
// .. -> parent dir
//      /api/abc/xyz/.. -> /api/abc              (match)
//      /api/abc/../xyz -> /api/xyz              (match)
//      /api/abc/.../xyz -> /api/abc/.../xyz     (not match)
//      /api/abc../xyz -> /api/abc../xyz         (not match)
//      /api/abc/..xyz -> /api/abc/..xyz         (not match)
func cleanPath(origin string) string {
	if origin == "" {
		return "/"
	}
	var realpos int
	tmp := pool.GetBuffer()
	defer pool.PutBuffer(tmp)
	if origin[0] != '/' {
		tmp.Resize(uint32(len(origin) + 1))
		tmp.AppendByte('/')
		realpos = 1
	} else {
		tmp.Resize(uint32(len(origin)))
	}
	buf := tmp.Bytes()
	for i, v := range common.Str2byte(origin) {
		if v == '/' {
			if realpos == 0 || buf[realpos-1] != '/' {
				buf[realpos] = v
				realpos++
			}
			continue
		}
		if v == '.' {
			if buf[realpos-1] != '/' {
				buf[realpos] = v
				realpos++
				continue
			}
			if i == len(origin)-1 {
				if realpos > 1 {
					realpos--
				}
				break
			}
			if origin[i+1] == '/' {
				continue
			}
			if origin[i+1] == '.' {
				if i+1 == len(origin)-1 {
					if realpos > 1 {
						realpos--
						for {
							if buf[realpos-1] == '/' {
								break
							}
							realpos--
						}
					}
					break
				}
				if origin[i+2] == '/' {
					if realpos > 1 {
						realpos--
						for {
							if buf[realpos-1] == '/' {
								break
							}
							realpos--
						}
					}
					continue
				}
			}
		}
		buf[realpos] = v
		realpos++
	}
	if realpos > 1 && buf[realpos-1] == '/' {
		realpos--
	}
	return string(buf[:realpos])
}

func (r *router) Get(path string, handler http.HandlerFunc) {
	r.getTree.Set(cleanPath(path), handler)
}
func (r *router) Post(path string, handler http.HandlerFunc) {
	r.postTree.Set(cleanPath(path), handler)
}
func (r *router) Patch(path string, handler http.HandlerFunc) {
	r.patchTree.Set(cleanPath(path), handler)
}
func (r *router) Put(path string, handler http.HandlerFunc) {
	r.putTree.Set(cleanPath(path), handler)
}
func (r *router) Delete(path string, handler http.HandlerFunc) {
	r.putTree.Set(cleanPath(path), handler)
}
func (r *router) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if r.rewriteHandler != nil {
		newurl, ok := r.rewriteHandler(req.URL.Path, req.Method)
		if ok {
			req.URL.Path = newurl
		}
	}
	var handler http.HandlerFunc
	if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/src") {
		handler = r.srcHandler
	} else if req.Method == http.MethodOptions {
		handler = r.optionsHandler
	} else {
		var ok bool
		switch req.Method {
		case http.MethodGet:
			handler, ok = r.getTree.Get(req.URL.Path)
		case http.MethodPost:
			handler, ok = r.postTree.Get(req.URL.Path)
		case http.MethodPut:
			handler, ok = r.putTree.Get(req.URL.Path)
		case http.MethodPatch:
			handler, ok = r.patchTree.Get(req.URL.Path)
		case http.MethodDelete:
			handler, ok = r.deleteTree.Get(req.URL.Path)
		}
		if !ok {
			cleanurl := cleanPath(req.URL.Path)
			switch req.Method {
			case http.MethodGet:
				handler, _ = r.getTree.Get(cleanurl)
			case http.MethodPost:
				handler, _ = r.postTree.Get(cleanurl)
			case http.MethodPut:
				handler, _ = r.putTree.Get(cleanurl)
			case http.MethodPatch:
				handler, _ = r.patchTree.Get(cleanurl)
			case http.MethodDelete:
				handler, _ = r.deleteTree.Get(cleanurl)
			}
		}
	}
	if handler == nil {
		if r.notFoundHandler != nil {
			r.notFoundHandler(resp, req)
		} else {
			http.NotFound(resp, req)
		}
	} else {
		handler(resp, req)
	}
}
