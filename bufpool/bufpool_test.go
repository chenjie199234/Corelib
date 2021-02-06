package bufpool

import (
	"testing"
	"time"

	ctime "github.com/chenjie199234/Corelib/time"
)

type A struct {
	Name     string
	now      []ctime.Time
	duration time.Duration
}

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	now := time.Now()
	b.Append(&A{Name: "name", now: []ctime.Time{ctime.Time(now), ctime.Time(now.Add(time.Hour))}, duration: time.Hour})
	t.Log(b.String())
}
