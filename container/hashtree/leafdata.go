package hashtree

import (
	"unsafe"
)

type LeafData struct {
	Hstr  []byte
	Value unsafe.Pointer
}
