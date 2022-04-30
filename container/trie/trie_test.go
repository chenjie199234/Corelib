package trie

import (
	"testing"
)

func Test_Trie(t *testing.T) {
	tree := NewTrie[int]()
	tree.Set("login", 1)
	tree.Set("logout", 2)
	tree.Set("/api/v1/ping", 3)
	tree.Set("/api/v2/ping", 4)
	tree.Set("/api/v1/pong", 5)
	tree.Set("/api/v1/login", 6)
	t.Log(tree.GetAll())

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
}
