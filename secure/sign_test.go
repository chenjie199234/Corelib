package secure

import (
	"testing"
)

func Test_Sign(t *testing.T) {
	sign, _ := SignMake("123456789")
	if e := SignCheck("123456789", sign); e != nil {
		t.Fatal(e)
	}
	if e := SignCheck("1234567890", sign); e == nil {
		t.Fatal("should failed")
	}
}
