package error

import (
	"testing"
)

var testmsg = `"lala\"la"`

func Test_Error(t *testing.T) {
	e := MakeError(10, 400, testmsg)
	t.Log(e.Error())
}
