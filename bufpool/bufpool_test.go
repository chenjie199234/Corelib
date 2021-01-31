package bufpool

import (
	"os"
	"testing"
)

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	b.Append([]byte("123"))
	os.Stderr.Write(b.Bytes())
}
