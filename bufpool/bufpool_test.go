package bufpool

import (
	"errors"
	"testing"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	var s = time.Second
	b.AppendStdDuration(s)
	if b.String() != "1000000000" {
		panic("std duration error")
	}
	b.Reset()
	b.AppendStdDurations([]time.Duration{s})
	if b.String() != "[1000000000]" {
		panic("std durations error")
	}
	b.Reset()
	b.AppendStdDurationPointers([]*time.Duration{nil, &s})
	if b.String() != "[null,1000000000]" {
		panic("std durationpointers error")
	}
	b.Reset()
	var cs ctime.Duration = ctime.Duration(s)
	b.AppendDuration(cs)
	if b.String() != "1s" {
		panic("corelib duration error")
	}
	b.Reset()
	b.AppendDurations([]ctime.Duration{cs})
	if b.String() != "[\"1s\"]" {
		panic("corelib durations error")
	}
	b.Reset()
	b.AppendDurationPointers([]*ctime.Duration{nil, &cs})
	if b.String() != "[null,\"1s\"]" {
		panic("corelib durationpointers error")
	}
	b.Reset()
	stde := errors.New("stde")
	ce := cerror.MakeError(1, 500, "ce")
	b.AppendError(stde)
	if b.String() != "stde" {
		panic("std error error")
	}
	b.Reset()
	b.AppendError(ce)
	if b.String() != "{\"code\":1,\"msg\":\"ce\"}" {
		panic("corelib error error")
	}
	b.Reset()
	b.AppendErrors([]error{nil, stde, ce})
	if b.String() != "[null,\"stde\",{\"code\":1,\"msg\":\"ce\"}]" {
		panic("corelib errors error")
	}
	b.Reset()
}
