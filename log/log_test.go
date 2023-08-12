package log

import (
	"errors"
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/util/ctime"
)

type A struct {
	Name string
}

func Test_Log(t *testing.T) {
	trace = true
	kvs := make(map[string]interface{}, 10)
	kvs["0"] = "abc\"\"defg"
	now := time.Now().UTC()
	kvs["\"1"] = now
	kvs["2"] = &now
	kvs["3"] = []time.Time{now, now}
	kvs["4"] = []*time.Time{&now, nil, &now}
	d := time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond + 5*time.Microsecond + 6*time.Nanosecond
	kvs["11"] = d
	kvs["12"] = &d
	kvs["13"] = []time.Duration{d, d}
	kvs["14"] = []*time.Duration{&d, nil, &d}

	cd := ctime.Duration(d)
	kvs["21"] = cd
	kvs["22"] = &cd
	kvs["23"] = []ctime.Duration{cd, cd}
	kvs["24"] = []*ctime.Duration{&cd, nil, &cd}

	stde := errors.New("std \" error")
	ce := cerror.MakeError(100, 400, "corelib error")
	kvs["31"] = stde
	kvs["32"] = []error{stde, nil, stde}
	kvs["33"] = ce
	kvs["34"] = []error{ce, nil, ce}
	kvs["35"] = []error{stde, nil, ce}
	Error(InitTrace(nil, "", "test", "127.0.0.1", "POST", "/abc", 0), "fffffuck", nil)
}
