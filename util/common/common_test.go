package common

import (
	"bytes"
	"testing"
)

func Test_Common(t *testing.T) {
	var sempty string = ""
	var s string = "abc"
	var bnil []byte = nil
	var bempty []byte = []byte{}
	var b []byte = []byte{'a', 'b', 'c'}

	if !bytes.Equal(STB(s), b) {
		panic("not empty str to byte failed")
	}
	if !bytes.Equal(STB(sempty), bempty) {
		panic("empty str to byte failed")
	}

	if BTS(b) != s {
		panic("not empty byte to str failed")
	}
	if BTS(bempty) != sempty {
		panic("empty byte to str failed")
	}
	if BTS(bnil) != sempty {
		panic("empty byte to str failed")
	}
}
