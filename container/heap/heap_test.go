package heap

import (
	"testing"
)

func Test_Heap(t *testing.T) {
	h := NewHeap(func(a, b interface{}) bool {
		return a.(int) < b.(int)
	})
	h.Push(10)
	h.Push(80)
	h.Push(23)
	h.Push(3)
	h.Push(9)
	h.Push(7)
	if d, ok := h.PopRoot(); !ok || d.(int) != 3 {
		t.Fatal("should get 3,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 7 {
		t.Fatal("should get 7,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 9 {
		t.Fatal("should get 9,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 10 {
		t.Fatal("should get 10,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 23 {
		t.Fatal("should get 23,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 80 {
		t.Fatal("should get 80,but get:", d)
	}
	if _, ok := h.PopRoot(); ok {
		t.Fatal("should get nil")
	}
	h = NewHeap(func(a, b interface{}) bool {
		return a.(int) > b.(int)
	})
	h.Push(10)
	h.Push(80)
	h.Push(23)
	h.Push(3)
	h.Push(9)
	h.Push(7)
	if d, ok := h.PopRoot(); !ok || d.(int) != 80 {
		t.Fatal("should get 80,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 23 {
		t.Fatal("should get 23,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 10 {
		t.Fatal("should get 10,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 9 {
		t.Fatal("should get 9,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 7 {
		t.Fatal("should get 7,but get:", d)
	}
	if d, ok := h.PopRoot(); !ok || d.(int) != 3 {
		t.Fatal("should get 3,but get:", d)
	}
	if _, ok := h.PopRoot(); ok {
		t.Fatal("should get nil")
	}
}
