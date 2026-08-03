package secure

import (
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"sync"
)

type sha256Hasher struct {
	hash.Hash
}

type sha512Hasher struct {
	hash.Hash
}

var sha256pool = &sync.Pool{
	New: func() any {
		return &sha256Hasher{sha256.New()}
	},
}
var sha512pool = &sync.Pool{
	New: func() any {
		return &sha512Hasher{sha512.New()}
	},
}

func GetSha256() *sha256Hasher {
	sha256.New()
	return sha256pool.Get().(*sha256Hasher)
}
func PutSha256(h *sha256Hasher) {
	h.Reset()
	sha256pool.Put(h)
}
func GetSha512() *sha512Hasher {
	return sha512pool.Get().(*sha512Hasher)
}
func PutSha512(h *sha512Hasher) {
	h.Reset()
	sha512pool.Put(h)
}
