package web

import (
	"errors"
	"fmt"
	//"net/http"
	"testing"
	"time"
)

func Test_Server(t *testing.T) {
	engine, _ := NewWebServer(&ServerConfig{}, "testgroup", "testname")
	engine.Get("/ping", 100*time.Millisecond, handleroot)
	engine.Delete("/ping", 100*time.Millisecond, handleroot)
	engine.Post("/ping", 100*time.Millisecond, handleroot)
	engine.Patch("/ping", 100*time.Millisecond, handleroot)
	engine.Put("/ping", 100*time.Millisecond, handleroot)
	engine.StartWebServer("127.0.0.1:8080", nil)
}
func handleroot(ctx *Context) {
	fmt.Println("123")
	ctx.Abort(888, errors.New("123"))
}
