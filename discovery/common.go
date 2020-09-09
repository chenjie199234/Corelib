package discovery

func bkdrhash(nameip string, total uint64) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range nameip {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
