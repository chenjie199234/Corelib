package nobody

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Context struct {
	request  *http.Request
	response http.ResponseWriter
	params   map[string]string
	store    map[string]interface{}
	calls    []HandleFunc
}

//store related
func (c *Context) GetStore(key string) interface{} {
	if c.store == nil {
		return nil
	} else if v, ok := c.store[key]; !ok {
		return nil
	} else {
		return v
	}
}
func (c *Context) SetStore(key string, value interface{}) {
	if c.store == nil {
		c.store = make(map[string]interface{}, 2)
	}
	c.store[key] = value
}

//params related
func (c *Context) GetParamString(key string) (string, error) {
	if c.params == nil {
		return "", ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		return v, nil
	} else {
		return "", ErrParamsNil
	}
}
func (c *Context) GetParamByte(key string) ([]byte, error) {
	if c.params == nil {
		return nil, ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		return str2byte(v), nil
	} else {
		return nil, ErrParamsNil
	}
}
func (c *Context) GetParamFloat32(key string) (float32, error) {
	if v, e := c.GetParamFloat64(key); e != nil {
		return 0, e
	} else {
		return float32(v), nil
	}
}
func (c *Context) GetParamFloat64(key string) (float64, error) {
	if c.params == nil {
		return 0, ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		if v, e := strconv.ParseFloat(v, 64); e != nil {
			return 0, ErrParamsType
		} else {
			return v, nil
		}
	} else {
		return 0, ErrParamsNil
	}
}
func (c *Context) GetParamInt32(key string) (int32, error) {
	if v, e := c.GetParamInt64(key); e != nil {
		return 0, e
	} else {
		return int32(v), nil
	}
}
func (c *Context) GetParamInt64(key string) (int64, error) {
	if c.params == nil {
		return 0, ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		if v, e := strconv.ParseInt(v, 10, 64); e != nil {
			return 0, ErrParamsType
		} else {
			return v, nil
		}
	} else {
		return 0, ErrParamsNil
	}
}
func (c *Context) GetParamUint32(key string) (uint32, error) {
	if v, e := c.GetParamUint64(key); e != nil {
		return 0, e
	} else {
		return uint32(v), e
	}
}
func (c *Context) GetParamUint64(key string) (uint64, error) {
	if c.params == nil {
		return 0, ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		if v, e := strconv.ParseUint(v, 10, 64); e != nil {
			return 0, ErrParamsType
		} else {
			return v, nil
		}
	} else {
		return 0, ErrParamsNil
	}
}
func (c *Context) GetParamBool(key string) (bool, error) {
	if c.params == nil {
		return false, ErrParamsNil
	}
	if v, ok := c.params[key]; ok {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "true" {
			return true, nil
		} else if v == "false" {
			return false, nil
		} else {
			return false, ErrParamsType
		}
	} else {
		return false, ErrParamsNil
	}
}

//request related
func (c *Context) GetRemoteAddr() string {
	if ip := c.request.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	if ip := c.request.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	ip, _, _ := net.SplitHostPort(c.request.RemoteAddr)
	return ip
}
func (c *Context) GetMethod() string {
	return c.request.Method
}
func (c *Context) GetPath() string {
	return c.request.URL.Path
}
func (c *Context) GetHost() string {
	return c.request.Host
}
func (c *Context) GetAgent() string {
	return c.request.UserAgent()
}
func (c *Context) GetReferer() string {
	return c.request.Referer()
}
func (c *Context) GetLanguage() []string {
	return strings.Split(c.request.Header.Get("Accept-Language"), ",")
}
func (c *Context) GetAcceptType() []string {
	return strings.Split(c.request.Header.Get("Accept"), ",")
}
func (c *Context) GetContentType() string {
	return c.request.Header.Get("Content-Type")
}
func (c *Context) GetAcceptEncoding() []string {
	return strings.Split(c.request.Header.Get("Accept-Encoding"), ",")
}
func (c *Context) GetCookie(key string) *http.Cookie {
	if temp, e := c.request.Cookie(key); e != nil {
		return nil
	} else {
		return temp
	}
}
func (c *Context) GetCookies() []*http.Cookie {
	return c.request.Cookies()
}

func (c *Context) GetForm() (map[string]string, error) {
	form := make(map[string]string)
	var data string
	if c.request.URL.RawQuery == "" {
		//get form from body
		if tempdata, e := ioutil.ReadAll(c.request.Body); e != nil {
			c.request.Body.Close()
			return nil, e
		} else if c.request.ContentLength > 0 && int64(len(tempdata)) > c.request.ContentLength {
			//tempdata = tempdata[:c.request.ContentLength]
			data = byte2str(tempdata[:c.request.ContentLength])
		} else {
			data = byte2str(tempdata)
		}
		c.request.Body.Close()
	} else {
		//get form from url
		data = c.request.URL.RawQuery
	}
	lastPos := -1
	for i := range data {
		if data[i] == '&' {
			if vv := strings.TrimSpace(data[lastPos+1 : i]); len(vv) != 0 {
				if index := strings.Index(vv, "="); index > 0 && index != (len(vv)-1) {
					form[vv[:index]] = vv[index+1:]
				}
			}
			lastPos = i
		}
	}
	if lastPos != len(c.request.URL.RawQuery)-1 {
		if vv := strings.TrimSpace(data[lastPos+1:]); len(vv) != 0 {
			if index := strings.Index(vv, "="); index > 0 && index != len(vv)-1 {
				form[vv[:index]] = vv[index+1:]
			}
		}
	}
	return form, nil
}
func (c *Context) GetRaw() ([]byte, error) {
	var e error
	var data []byte
	if data, e = ioutil.ReadAll(c.request.Body); e != nil {
		c.request.Body.Close()
		return nil, e
	} else if c.request.ContentLength != -1 && c.request.ContentLength != 0 && int64(len(data)) > c.request.ContentLength {
		data = data[:c.request.ContentLength]
	}
	c.request.Body.Close()
	return data, nil
}

type Single struct {
	Name        string
	FileName    string
	ContentType string
	Body        []byte
}

func (c *Context) GetMulti() ([]*Single, error) {
	totalCT := strings.Split(c.request.Header.Get("Content-Type"), ";")
	if len(totalCT) < 2 {
		return nil, nil
	}
	totalCT[0] = strings.TrimSpace(totalCT[0])
	totalCT[1] = strings.TrimSpace(totalCT[1])
	if strings.HasPrefix(totalCT[0], "multipart/") && strings.HasPrefix(totalCT[1], "boundary=") {
		body := make([]*Single, 0)
		var err error
		mr := multipart.NewReader(c.request.Body, totalCT[1][9:])
		for {
			if p, e := mr.NextPart(); e != nil {
				if e == io.EOF {
					break
				} else {
					err = e
					break
				}
			} else {
				tempbody := new(Single)
				if tempbody.Body, e = ioutil.ReadAll(p); e != nil {
					continue
				}
				if tempbody.ContentType = strings.TrimSpace(p.Header.Get("Content-Type")); tempbody.ContentType == "" {
					tempbody.ContentType = TextPlain
				}
				tempbody.Name = p.FormName()
				tempbody.FileName = p.FileName()
				body = append(body, tempbody)
			}
		}
		return body, err
	} else {
		return nil, nil
	}
}

//response related
func (c *Context) SetStatusCode(code int) {
	c.response.WriteHeader(code)
}
func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.response, cookie)
}

//text
func (c *Context) WriteString(data string, compress bool) {
	c.Write(str2byte(data), compress)
}
func (c *Context) Write(data []byte, compress bool) {
	c.response.Header().Set("Content-Type", TextPlain)
	c.dealRawData(data, compress)
}

//json
func (c *Context) WriteJsonString(data string, compress bool) {
	c.WriteJson(str2byte(data), compress)
}
func (c *Context) WriteJson(data []byte, compress bool) {
	c.response.Header().Set("Content-Type", ApplicationJson)
	c.dealRawData(data, compress)
}

//protobuf
func (c *Context) WritePbString(data string, compress bool) {
	c.WriteJson(str2byte(data), compress)
}
func (c *Context) WritePb(data []byte, compress bool) {
	c.response.Header().Set("Content-Type", ApplicationProtobuf)
	c.dealRawData(data, compress)
}

//msgpack
func (c *Context) WriteMpString(data string, compress bool) {
	c.WriteMp(str2byte(data), compress)
}
func (c *Context) WriteMp(data []byte, compress bool) {
	c.response.Header().Set("Content-Type", ApplicationMsgpack)
	c.dealRawData(data, compress)
}
func (c *Context) dealRawData(data []byte, compress bool) {
	if len(data) == 0 {
		return
	}
	if compress {
		c.response.Header().Set("Content-Encoding", "gzip")
		if c.GetMethod() == http.MethodHead {
			return
		}
		gw := poolInstance.getGzipWriter(c.response)
		gw.Write(data)
		gw.Flush()
		poolInstance.putGzipWriter(gw)
	} else {
		if c.GetMethod() == http.MethodHead {
			return
		}
		c.response.Write(data)
	}
}

//source
func (c *Context) WriteSource(path string, compress bool) {
	if path[len(path)-1] == '/' {
		if path[0] == '/' {
			path = path[1 : len(path)-1]
		} else {
			path = path[:len(path)-1]
		}
	} else {
		if path[0] == '/' {
			path = path[1:]
		}
	}
	path, _ = filepath.Abs(path)
	switch filepath.Ext(path) {
	case ".html":
		c.writeHtml(path, compress)
	case ".js":
		c.writeJs(path, compress)
	case ".css":
		c.writeCss(path, compress)
	case ".jpeg":
		//.jpeg -> .jpg
		path = path[:len(path)-2] + "g"
		fallthrough
	case ".jpg":
		c.writeJpg(path, compress)
	case ".png":
		c.writePng(path, compress)
	case ".webp":
		c.writeWebp(path, compress)
	case ".ico":
		c.writeIco(path, compress)
	default:
		http.Error(c.response, "404 source not found", http.StatusNotFound)
	}
}

//html abs path without '/' at the end
func (c *Context) writeHtml(path string, compress bool) {
	c.response.Header().Set("Content-Type", TextHtml)
	c.dealSrcData(path, compress)
}

//css abs path without '/' at the end
func (c *Context) writeCss(path string, compress bool) {
	c.response.Header().Set("Content-Type", TextCss)
	c.dealSrcData(path, compress)
}

//js abs path without '/' at the end
func (c *Context) writeJs(path string, compress bool) {
	c.response.Header().Set("Content-Type", ApplicationJS)
	c.dealSrcData(path, compress)
}

//jpg abs path without '/' at the end
func (c *Context) writeJpg(path string, compress bool) {
	c.response.Header().Set("Content-Type", ImageJpg)
	c.dealSrcData(path, compress)
}

//png abs path without '/' at the end
func (c *Context) writePng(path string, compress bool) {
	c.response.Header().Set("Content-Type", ImagePng)
	c.dealSrcData(path, compress)
}

//webp abs path without '/' at the end
func (c *Context) writeWebp(path string, compress bool) {
	c.response.Header().Set("Content-Type", ImageWebp)
	c.dealSrcData(path, compress)
}

//ico abs path without '/' at the end
func (c *Context) writeIco(path string, compress bool) {
	c.response.Header().Set("Content-Type", ImageIcon)
	c.dealSrcData(path, compress)
}
func (c *Context) dealSrcData(path string, compress bool) {
	if compress {
		needCompress := false
		f, e := os.Open(path + ".gz")
		if e != nil {
			f, e = os.Open(path)
			if e != nil {
				http.Error(c.response, "404 source not found", http.StatusNotFound)
				return
			}
			needCompress = true
		}
		finfo, e := f.Stat()
		if e != nil || finfo.Size() == 0 {
			http.Error(c.response, "404 source not found", http.StatusNotFound)
			f.Close()
			return
		}
		same, etag := c.checkCache(finfo)
		if c.GetMethod() == http.MethodHead {
			c.response.Header().Set("Content-Encoding", "gzip")
			c.response.Header().Set("Etag", etag)
			c.response.Header().Set("Cache-Control", "no-cache")
			f.Close()
			return
		}
		if same {
			c.response.Header().Del("Content-Type")
			c.response.WriteHeader(http.StatusNotModified)
		} else {
			c.response.Header().Set("Content-Encoding", "gzip")
			c.response.Header().Set("Etag", etag)
			c.response.Header().Set("Cache-Control", "no-cache")
			if needCompress {
				gw := poolInstance.getGzipWriter(c.response)
				io.Copy(gw, f)
				gw.Flush()
				poolInstance.putGzipWriter(gw)
			} else {
				io.Copy(c.response, f)
			}
		}
		f.Close()
	} else {
		f, e := os.Open(path)
		if e != nil {
			http.Error(c.response, "404 source not found", http.StatusNotFound)
			return
		}
		finfo, e := f.Stat()
		if e != nil || finfo.Size() == 0 {
			http.Error(c.response, "404 source not found", http.StatusNotFound)
			f.Close()
			return
		}
		same, etag := c.checkCache(finfo)
		if c.GetMethod() == http.MethodHead {
			c.response.Header().Set("Etag", etag)
			c.response.Header().Set("Cache-Control", "no-cache")
			f.Close()
			return
		}
		if same {
			c.response.Header().Del("Content-Type")
			c.response.WriteHeader(http.StatusNotModified)
		} else {
			c.response.Header().Set("Etag", etag)
			c.response.Header().Set("Cache-Control", "no-cache")
			io.Copy(c.response, f)
		}
		f.Close()
	}
}
func (c *Context) checkCache(finfo os.FileInfo) (bool, string) {
	newEtag := fmt.Sprintf("%d%d", finfo.Size(), finfo.ModTime().Unix())
	//get etag
	etag := c.request.Header.Get("If-None-Match")
	if etag == "" {
		etag = c.request.Header.Get("If-Match")
		if etag == "" {
			return false, newEtag
		}
	}
	//check etag
	if newEtag == etag {
		return true, etag
	} else {
		return false, newEtag
	}
}

//download
func (c *Context) WriteDownload(path string) {
	if path[len(path)-1] == '/' {
		if path[0] == '/' {
			path = path[1 : len(path)-1]
		} else {
			path = path[:len(path)-1]
		}
	} else {
		if path[0] == '/' {
			path = path[1:]
		}
	}
	path, _ = filepath.Abs(path)
	switch filepath.Ext(path) {
	case ".zip":
		c.writeZip(path)
	case ".rar":
		c.writeRar(path)
	case ".7z":
		c.write7z(path)
	default:
		c.writeBin(path)
	}
}

//7z abs path without '/' at the end
func (c *Context) write7z(path string) {
	c.response.Header().Set("Content-Type", Compress7z)
	c.response.Header().Set("Content-Disposition", "attachment;filename="+filepath.Base(path))
	c.dealDownData(path)
}

//zip abs path without '/' at the end
func (c *Context) writeZip(path string) {
	c.response.Header().Set("Content-Type", CompressZip)
	c.response.Header().Set("Content-Disposition", "attachment;filename="+filepath.Base(path))
	c.dealDownData(path)
}

//rar abs path without '/' at the end
func (c *Context) writeRar(path string) {
	c.response.Header().Set("Content-Type", CompressRar)
	c.response.Header().Set("Content-Disposition", "attachment;filename="+filepath.Base(path))
	c.dealDownData(path)
}

//binary abc path without '/' at the end
func (c *Context) writeBin(path string) {
	c.response.Header().Set("Content-Type", ApplicationBin)
	c.response.Header().Set("Content-Disposition", "attachment;filename="+filepath.Base(path))
	c.dealDownData(path)
}
func (c *Context) dealDownData(path string) {
	f, e := os.Open(path)
	if e != nil {
		c.response.Header().Del("Content-Disposition")
		http.Error(c.response, "404 source not found", http.StatusNotFound)
		return
	}
	finfo, e := f.Stat()
	if e != nil || finfo.Size() == 0 {
		c.response.Header().Del("Content-Disposition")
		http.Error(c.response, "404 source not found", http.StatusNotFound)
		f.Close()
		return
	}
	if c.GetMethod() == http.MethodHead {
		c.response.Header().Set("Accept-Ranges", "bytes")
		c.response.Header().Set("Content-Length", fmt.Sprintf("%d", finfo.Size()))
		f.Close()
		return
	}
	parts, etag := c.checkRange(finfo)
	if etag == "" {
		c.response.Header().Del("Content-Disposition")
		http.Error(c.response, "416 invalid range", http.StatusRequestedRangeNotSatisfiable)
		f.Close()
		return
	}
	c.response.Header().Set("Accept-Ranges", "bytes")
	c.response.Header().Set("Etag", etag)
	if parts == nil || len(parts) == 0 {
		c.response.Header().Set("Content-Length", fmt.Sprintf("%d", finfo.Size()))
		c.response.WriteHeader(http.StatusOK)
		io.Copy(c.response, f)
	} else if len(parts)/2 == 1 {
		if _, e = f.Seek(parts[0], io.SeekStart); e != nil {
			c.response.Header().Del("Accept-Ranges")
			c.response.Header().Del("Content-Disposition")
			c.response.Header().Del("Etag")
			http.Error(c.response, "500 seek failed", http.StatusInternalServerError)
		} else {
			c.response.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", parts[0], parts[1], finfo.Size()))
			c.response.WriteHeader(http.StatusPartialContent)
			io.CopyN(c.response, f, parts[1]-parts[0]+1)
		}
	} else {
		boundary := fmt.Sprintf("%x", rand.Int63())
		oldtype := c.response.Header().Get("Content-Type")
		c.response.Header().Set("Content-Type", "multipart/byteranges;boundary="+boundary)
		c.response.WriteHeader(http.StatusPartialContent)
		for i := 0; i < len(parts); i += 2 {
			if _, e := f.Seek(parts[i], io.SeekStart); e != nil {
				f.Close()
				return
			}
			fmt.Fprintf(c.response, "--%s\r\n", boundary)
			fmt.Fprintf(c.response, "Content-Type: %s\r\n", oldtype)
			fmt.Fprintf(c.response, "Content-Range: %d-%d/%d\r\n", parts[i], parts[i+1], finfo.Size())
			fmt.Fprintf(c.response, "\r\n")
			if n, e := io.CopyN(c.response, f, parts[i+1]-parts[i]+1); e != nil && e != io.EOF || n == 0 {
				f.Close()
				return
			}
			fmt.Fprintf(c.response, "\r\n")
		}
		fmt.Fprintf(c.response, "--%s--\r\n", boundary)
	}
	f.Close()
}
func (c *Context) checkRange(finfo os.FileInfo) ([]int64, string) {
	newEtag := fmt.Sprintf("%d%d", finfo.Size(), finfo.ModTime().Unix())
	etag := c.request.Header.Get("If-Range")
	if etag != "" && etag != newEtag {
		//old and new etag different,download from the begining
		return nil, newEtag
	}
	//same etag
	range_str := strings.TrimSpace(c.request.Header.Get("Range"))
	if range_str == "" {
		return nil, newEtag
	} else if len(range_str) < 8 {
		//smallest range_str:bytes=0-
		return nil, ""
	} else if !strings.HasPrefix(range_str, "bytes=") {
		return nil, ""
	}
	result := make([]int64, 0)
	var start int64
	var end int64
	var e error
	for _, v := range strings.Split(range_str[6:], ",") {
		if v == "" || v == "-" {
			continue
		}
		fi := strings.Index(v, "-")
		li := strings.LastIndex(v, "-")
		if fi != li || fi == -1 || li == -1 {
			return nil, ""
		}
		if fi == 0 {
			//-xxx
			end = finfo.Size() - 1
			var temp int64
			if temp, e = strconv.ParseInt(v[fi+1:], 10, 64); e != nil {
				return nil, ""
			} else {
				start = finfo.Size() - temp
			}
		} else if fi == len(v)-1 {
			//xxx-
			end = finfo.Size() - 1
			if start, e = strconv.ParseInt(v[:fi], 10, 64); e != nil {
				return nil, ""
			}
		} else {
			//xxx-xxx
			if start, e = strconv.ParseInt(v[:fi], 10, 64); e != nil {
				return nil, ""
			}
			if end, e = strconv.ParseInt(v[fi+1:], 10, 64); e != nil {
				return nil, ""
			}
		}
		if start > end {
			return nil, ""
		} else if start > finfo.Size()-1 || end > finfo.Size()-1 {
			return nil, ""
		} else {
			result = append(result, start, end)
		}
	}
	return result, newEtag
}

//end
func (c *Context) reset() {
	c.params = make(map[string]string)
	c.store = make(map[string]interface{})
	c.calls = nil
}
