package discovery

import (
	"github.com/chenjie199234/Corelib/stream"
)

type node struct {
	p *stream.Peer
	n string
	u uint64
}

func bkdrhash(nameip string, total uint64) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range nameip {
		hash = hash*seed + uint64(v)
	}
	return hash % total
}
