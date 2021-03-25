package web

import (
	"fmt"
	//"net/http"
	"testing"
	"time"
)

func Test_Server(t *testing.T) {
	engine, _ := NewWebServer(&ServerConfig{}, "testgroup", "testname")
	engine.Get("/ping", 100*time.Millisecond, handleroot)
	engine.StartWebServer("127.0.0.1:8080", nil)
}
func handleroot(ctx *Context) {
	fmt.Println("123")
	ctx.WriteString(888, "123")
}
