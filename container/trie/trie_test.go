package trie

import (
	"fmt"
	"strings"
	"testing"
)

func Test_Trie(t *testing.T) {
	tree := NewTrie[int]()
	tree.Set("aaa", 0)
	tree.Set("login", 1)
	tree.Set("logout", 2)
	tree.Set("/api/v1/ping", 3)
	tree.Set("/api/v2/ping", 4)
	tree.Set("/api/v1/pong", 5)
	tree.Set("/api/v1/login", 6)
	tree.Set("/api/v1/pi", 7)
	for _, v := range *tree {
		p(0, v)
	}

	data, ok := tree.Get("login")
	if !ok {
		panic("should be ok")
	}
	if data != 1 {
		panic("value wrong")
	}

	data, ok = tree.Get("logout")
	if !ok {
		panic("should be ok")
	}
	if data != 2 {
		panic("value wrong")
	}

	data, ok = tree.Get("/api/v1/ping")
	if !ok {
		panic("should be ok")
	}
	if data != 3 {
		panic("value wrong")
	}
	data, ok = tree.Get("/api/v2/ping")
	if !ok {
		panic("should be ok")
	}
	if data != 4 {
		panic("value wrong")
	}
	data, ok = tree.Get("/api/v1/pong")
	if !ok {
		panic("should be ok")
	}
	if data != 5 {
		panic("value wrong")
	}

	data, ok = tree.Get("/api/v1/login")
	if !ok {
		panic("should be ok")
	}
	if data != 6 {
		panic("value wrong")
	}

	data, ok = tree.Get("/api")
	if ok {
		panic("should be not ok")
	}
	if tree.Del("/api/v1") {
		panic("should not be ok")
	}
	if tree.Del("/api/v1/") {
		panic("should not be ok")
	}
	if !tree.Del("/api/v1/pi") {
		panic("should be ok")
	}
	if !tree.Del("login") {
		panic("should be ok")
	}
	if !tree.Del("aaa") {
		panic("should be ok")
	}
	if _, ok := tree.Get("/api/v1/pi"); ok {
		panic("already deleted,should not be ok")
	}
	if data, ok := tree.Get("/api/v1/pong"); !ok {
		panic("should be ok")
	} else if data != 5 {
		panic("value wrong")
	}
	for _, v := range *tree {
		p(0, v)
	}
}
func p(space int, n *node[int]) {
	fmt.Println(strings.Repeat(" ", space) + n.str)
	for _, child := range n.children {
		p(space+len(n.str), child)
	}
}
