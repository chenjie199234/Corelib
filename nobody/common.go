package nobody

import (
	"errors"
	"math/rand"
	"net"
	"net/http"
	"time"
	"unsafe"
)

//content-type
var (
	//default
	TextPlain      = "text/plain"
	ApplicationBin = "application/octet-stream"

	//source
	TextHtml      = "text/html"
	TextCss       = "text/css"
	ApplicationJS = "application/javascript"

	//image
	ImageWebp = "image/webp"
	ImageJpg  = "image/jpeg"
	ImagePng  = "image/png"
	ImageIcon = "image/x-icon"

	//data
	ApplicationJson     = "application/json"
	ApplicationProtobuf = "application/protobuf"
	ApplicationMsgpack  = "application/msgpack"

	//download file
	Compress7z  = "application/x-7z-compressed"
	CompressZip = "application/zip"
	CompressRar = "application/x-rar-compressed"
)

var (
	ErrParamsNil  = errors.New("Param:not exist")
	ErrParamsType = errors.New("Param:wrong type")
)

type HandleFunc func(ctx *Context)
type ConnStateFunc func(c net.Conn, s http.ConnState)
type ShutdownFunc func()

func init() {
	rand.Seed(time.Now().Unix())
}
func str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
