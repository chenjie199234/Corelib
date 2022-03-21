package hashtree

type LeafData[T any] struct {
	Hstr  []byte
	Value T
}
