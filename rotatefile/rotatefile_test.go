package rotatefile

import (
	"bytes"
	"testing"
)

func Test_RotateFile(t *testing.T) {
	f, e := NewRotateFile("./", "abc", 1024)
	if e != nil {
		panic(e)
	}
	for i := 0; i < 641; i++ {
		if e := f.Write(bytes.Repeat([]byte{'a'}, 15)); e != nil {
			panic(e)
		}
		if e := f.Write([]byte{'\n'}); e != nil {
			panic(e)
		}
	}
	f.Close(true)
	if e := f.Write([]byte{'\n'}); e == nil {
		panic("file already closed,write should return error,but no error return")
	}
}
