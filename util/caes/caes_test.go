package caes

import (
	// "encoding/hex"
	"testing"
)

func Test_Aes(t *testing.T) {
	d, e := Encrypt("123456789", []byte("abcdefgh1111111111111111"))
	if e != nil {
		t.Fatal(e)
	}
	origin, e := Decrypt("123456789", d)
	if e != nil {
		t.Fatal(e)
	}
	if string(origin) != "abcdefgh1111111111111111" {
		t.Fatal("data broken")
	}
}
