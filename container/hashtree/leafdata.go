package hashtree

type LeafData[T any] struct {
	Hstr  []byte
	Value T
}

func getNodeStartIndexInSelfPiece(index, width int) int {
	return (((index-1)/width)*width + 1)
}
func getNodeStartIndexInChildPiece(index, width int) int {
	return index*width + 1
}
func getparentindex(index, width int) int {
	sindex := getNodeStartIndexInSelfPiece(index, width)
	return (sindex - 1) / width
}
