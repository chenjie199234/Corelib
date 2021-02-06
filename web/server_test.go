package web

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func Test_Server(t *testing.T) {
	engine := NewWebServer(&Config{})
	engine.Post("/ping", 100*time.Millisecond, handleroot)
	engine.StartWebServer("127.0.0.1:8080", "", "")
}
func handleroot(ctx *Context) {
	fmt.Println("123")
	ctx.WriteString(http.StatusOK, "123")
}
