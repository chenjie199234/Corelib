package web

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chenjie199234/Corelib/container/trie"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
)

type router struct {
	getTree    *trie.Trie[http.HandlerFunc]
	postTree   *trie.Trie[http.HandlerFunc]
	putTree    *trie.Trie[http.HandlerFunc]
	patchTree  *trie.Trie[http.HandlerFunc]
	deleteTree *trie.Trie[http.HandlerFunc]
	//first key method,second key old path,value new path
	rewrite map[string]map[string]string
	//first key method,second key path,value timeout,value <= 0 means no timeout
	//first key:'default',second key:'default',the value is the default timeout
	timeout              map[string]map[string]time.Duration
	notFoundHandler      http.HandlerFunc
	srcPermissionHandler http.HandlerFunc
	optionsHandler       http.HandlerFunc
	srcroot              fs.FS
}

func newRouter(srcroot string) *router {
	r := &router{
		getTree:    trie.NewTrie[http.HandlerFunc](),
		postTree:   trie.NewTrie[http.HandlerFunc](),
		putTree:    trie.NewTrie[http.HandlerFunc](),
		patchTree:  trie.NewTrie[http.HandlerFunc](),
		deleteTree: trie.NewTrie[http.HandlerFunc](),
		rewrite:    make(map[string]map[string]string),
		timeout:    make(map[string]map[string]time.Duration),
	}
	if srcroot != "" {
		r.srcroot = os.DirFS(srcroot)
	}
	return r
}

func (r *router) UpdateSrcRoot(srcroot string) {
	if srcroot == "" {
		r.srcroot = nil
	} else {
		r.srcroot = os.DirFS(srcroot)
	}
}

// the first character must be slash(/)
//
//	api/abc -> /api/abc
//
// remove tail slash(/)
//
//	/api/abc/ -> /api/abc
//
// multi series slash(///) -> single slash(/)
//
//	/api//abc -> /api/abc
//
// . -> current dir
//
//	/api/abc/. -> /api/abc                   (match)
//	/api/./abc -> /api/abc                   (match)
//	/api./abc  -> /api./abc                  (not match)
//	/api/.abc  -> /api/.abc                  (not match)
//
// .. -> parent dir
//
//	/api/abc/xyz/.. -> /api/abc              (match)
//	/api/abc/../xyz -> /api/xyz              (match)
//	/api/abc/.../xyz -> /api/abc/.../xyz     (not match)
//	/api/abc../xyz -> /api/abc../xyz         (not match)
//	/api/abc/..xyz -> /api/abc/..xyz         (not match)
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

// first key method,second key old path,value new path
func (r *router) updaterewrite(rewrite map[string]map[string]string) {
	if rewrite == nil {
		r.rewrite = make(map[string]map[string]string)
	} else {
		r.rewrite = rewrite
	}
}

// first key method,second key path,value timeout,value <= 0 means no timeout
// first key:'default',second key:'default',the value is the default timeout
func (r *router) updatetimeout(timeout map[string]map[string]time.Duration) {
	if timeout == nil {
		r.timeout = make(map[string]map[string]time.Duration)
	} else {
		r.timeout = timeout
	}
}

func (r *router) checkrewrite(oldpath, method string) (newpath string, ok bool) {
	rewrite := r.rewrite
	paths, ok := rewrite[method]
	if !ok {
		return
	}
	newpath, ok = paths[oldpath]
	return
}
func (r *router) checktimeout(path, method string) time.Duration {
	timeout := r.timeout
	paths, ok := timeout[method]
	if !ok {
		if paths, ok = timeout["default"]; !ok {
			return 0
		}
		if v, ok := paths["default"]; ok {
			return v
		}
		return 0
	}
	if v, ok := paths[path]; ok {
		return v
	} else if v, ok = paths["default"]; ok {
		return v
	}
	return 0
}
func (r *router) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if url, ok := r.checkrewrite(req.URL.Path, req.Method); ok {
		req.URL.Path = url
	}
	if timeout := r.checktimeout(req.URL.Path, req.Method); timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}
	var handler http.HandlerFunc
	var cleanurl string
	if req.Method == http.MethodOptions {
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
			cleanurl = cleanPath(req.URL.Path)
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
	if handler != nil {
		handler(resp, req)
	} else if r.srcroot == nil {
		r.notFoundHandler(resp, req)
	} else if req.Method != http.MethodGet {
		r.notFoundHandler(resp, req)
	} else {
		//src root exist,no api handler,the request is GET,try to serve static resource
		if cleanurl == "/" {
			cleanurl = "/index.html"
		}
		srcroot := r.srcroot
		file, e := srcroot.Open(cleanurl[1:])
		if e != nil {
			if os.IsNotExist(e) {
				r.notFoundHandler(resp, req)
			} else {
				r.srcPermissionHandler(resp, req)
			}
		} else if fileinfo, e := file.Stat(); e != nil || !fileinfo.Mode().IsRegular() {
			r.notFoundHandler(resp, req)
			file.Close()
		} else {
			http.ServeContent(resp, req, filepath.Base(cleanurl), fileinfo.ModTime(), file.(*os.File))
			file.Close()
		}
	}
}
func (r *router) printPath() {
	for path := range r.getTree.GetAll() {
		log.Info(nil, "[web.server] GET: "+path, nil)
	}
	if rewrite, ok := r.rewrite["GET"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] GET: "+ourl+" => "+nurl, nil)
		}
	}
	for path := range r.postTree.GetAll() {
		log.Info(nil, "[web.server] POST: "+path, nil)
	}
	if rewrite, ok := r.rewrite["POST"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] POST: "+ourl+" => "+nurl, nil)
		}
	}
	for path := range r.putTree.GetAll() {
		log.Info(nil, "[web.server] PUT: "+path, nil)
	}
	if rewrite, ok := r.rewrite["PUT"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] PUT: "+ourl+" => "+nurl, nil)
		}
	}
	for path := range r.patchTree.GetAll() {
		log.Info(nil, "[web.server] PATCH: "+path, nil)
	}
	if rewrite, ok := r.rewrite["PATCH"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] PATCH: "+ourl+" => "+nurl, nil)
		}
	}
	for path := range r.deleteTree.GetAll() {
		log.Info(nil, "[web.server] DELETE: "+path, nil)
	}
	if rewrite, ok := r.rewrite["DELETE"]; ok {
		for ourl, nurl := range rewrite {
			log.Info(nil, "[web.server] DELETE: "+ourl+" => "+nurl, nil)
		}
	}
}
