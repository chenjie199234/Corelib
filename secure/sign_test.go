package secure

import (
	"strings"
	"testing"
)

func Test_Sign(t *testing.T) {
	sign, _ := SignMake(strings.Repeat("s", 32))
	if e := SignCheck(strings.Repeat("s", 32), sign); e != nil {
		t.Fatal(e)
	}
	if e := SignCheck(strings.Repeat("s", 31), sign); e == nil {
		t.Fatal("should failed")
	}
}
