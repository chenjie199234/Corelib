package web

import (
	"fmt"
	"net/http"
	"testing"
)

func Test_Server(t *testing.T) {
	instance := NewInstance(&WebConfig{
		Addr:                 "127.0.0.1:9234",
		ReadHeaderTimeout:    200,
		ReadTimeout:          500,
		WriteTimeout:         500,
		IdleTimeout:          1000,
		MaxHeaderBytes:       1024,
		SocketReadBufferLen:  1024,
		SocketWriteBufferLen: 1024,
	})
	instance.GET("/", handleroot)
	instance.StartWebServer()
}
func handleroot(ctx *Context) {
	<-ctx.Done()
	fmt.Println("123")
	ctx.WriteString(http.StatusOK, "123")
}
