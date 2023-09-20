package secure

import (
	"bytes"
	"testing"
)

func Test_Aes(t *testing.T) {
	plaintxt := []byte("abcdefg")
	chipertxt, _ := AesEncrypt("123456789", plaintxt)
	newplaintxt, e := AesDecrypt("123456789", chipertxt)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(plaintxt, newplaintxt) {
		t.Fatal("should same")
	}
	if _, e := AesDecrypt("1234567890", chipertxt); e == nil {
		t.Fatal("should failed")
	}
}
