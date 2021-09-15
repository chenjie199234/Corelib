package bufpool

import (
	"errors"
	"testing"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	ctime "github.com/chenjie199234/Corelib/util/time"
	//"time"
)

type A struct {
	E1 *error
	E2 error
	E3 error
	E4 *cerror.Error
	E5 error
	T1 ctime.Time
}

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	e := errors.New("e")
	ee := cerror.Error{}
	b.Append(&A{E1: &e, E2: e, E3: ee, E4: &ee, E5: nil, T1: ctime.Time(time.Now())})
	t.Log(b.String())
}
