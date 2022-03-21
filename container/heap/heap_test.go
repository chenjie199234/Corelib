package heap

import (
	"testing"
)

type ts struct {
	money int
}

func Test_Heap(t *testing.T) {
	h1 := NewHeap(func(a, b *ts) bool {
		return a.money < b.money
	})
	h1.Push(&ts{10})
	h1.Push(&ts{80})
	h1.Push(&ts{23})
	h1.Push(&ts{3})
	h1.Push(&ts{7})
	h1.Push(&ts{9})
	if d, ok := h1.PopRoot(); !ok || d.money != 3 {
		t.Fatal("should get 3,but get:", d)
	}
	if d, ok := h1.PopRoot(); !ok || d.money != 7 {
		t.Fatal("should get 7,but get:", d)
	}
	if d, ok := h1.PopRoot(); !ok || d.money != 9 {
		t.Fatal("should get 9,but get:", d)
	}
	if d, ok := h1.PopRoot(); !ok || d.money != 10 {
		t.Fatal("should get 10,but get:", d)
	}
	if d, ok := h1.PopRoot(); !ok || d.money != 23 {
		t.Fatal("should get 23,but get:", d)
	}
	if d, ok := h1.PopRoot(); !ok || d.money != 80 {
		t.Fatal("should get 80,but get:", d)
	}
	if _, ok := h1.PopRoot(); ok {
		t.Fatal("should get nil")
	}

	h2 := NewHeap(func(a, b int) bool {
		return a > b
	})
	h2.Push(10)
	h2.Push(80)
	h2.Push(23)
	h2.Push(3)
	h2.Push(9)
	h2.Push(7)
	if d, ok := h2.PopRoot(); !ok || d != 80 {
		t.Fatal("should get 80,but get:", d)
	}
	if d, ok := h2.PopRoot(); !ok || d != 23 {
		t.Fatal("should get 23,but get:", d)
	}
	if d, ok := h2.PopRoot(); !ok || d != 10 {
		t.Fatal("should get 10,but get:", d)
	}
	if d, ok := h2.PopRoot(); !ok || d != 9 {
		t.Fatal("should get 9,but get:", d)
	}
	if d, ok := h2.PopRoot(); !ok || d != 7 {
		t.Fatal("should get 7,but get:", d)
	}
	if d, ok := h2.PopRoot(); !ok || d != 3 {
		t.Fatal("should get 3,but get:", d)
	}
	if _, ok := h2.PopRoot(); ok {
		t.Fatal("should get nil")
	}
}
