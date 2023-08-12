package pool

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/ctime"
)

func Test_Pool(t *testing.T) {
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
	if b.String() != "0h0m1s0ms0us0ns" {
		panic("corelib duration error")
	}
	b.Reset()
	b.AppendDurations([]ctime.Duration{cs})
	if b.String() != "[\"0h0m1s0ms0us0ns\"]" {
		panic("corelib durations error")
	}
	b.Reset()
	b.AppendDurationPointers([]*ctime.Duration{nil, &cs})
	if b.String() != "[null,\"0h0m1s0ms0us0ns\"]" {
		panic("corelib durationpointers error")
	}
	b.Reset()
	normale := cerror.MakeError(1, 500, "normale")
	speciale := cerror.MakeError(1, 500, "special\"e")
	b.AppendError(normale)
	if b.String() != "{\"code\":1,\"msg\":\"normale\"}" {
		panic("corelib error error")
	}
	b.Reset()
	b.AppendError(speciale)
	t.Log(b.String())
	if b.String() != "{\"code\":1,\"msg\":\"special\\\"e\"}" {
		panic("corelib error error")
	}
	b.Reset()
	b.AppendErrors([]*cerror.Error{nil, normale, speciale})
	t.Log(b.String())
	if b.String() != "[null,{\"code\":1,\"msg\":\"normale\"},{\"code\":1,\"msg\":\"special\\\"e\"}]" {
		panic("corelib errors error")
	}
	b.Reset()
}
