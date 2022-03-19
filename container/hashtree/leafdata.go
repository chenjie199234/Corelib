package hashtree

import (
	"unsafe"
)

type LeafData struct {
	Hashstr []byte
	Value   unsafe.Pointer
}
