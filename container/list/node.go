package list

import (
	"unsafe"
)

type node struct {
	value unsafe.Pointer
	next  *node
}
