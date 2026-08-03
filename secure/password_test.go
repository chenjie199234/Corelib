package secure

import (
	"strings"
	"testing"
)

func Test_Password(t *testing.T) {
	sign, _ := SignPassword(strings.Repeat("s", 32))
	if e := CheckPasswordSign(strings.Repeat("s", 32), sign); e != nil {
		t.Fatal(e)
	}
	if e := CheckPasswordSign(strings.Repeat("s", 31), sign); e == nil {
		t.Fatal("should failed")
	}
}
