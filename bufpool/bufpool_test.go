package bufpool

import (
	"testing"
	"time"

	mtime "github.com/chenjie199234/Corelib/time"
)

type A struct {
	Name     string
	now      []mtime.Time
	duration time.Duration
}

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	now := time.Now()
	b.Append(&A{Name: "name", now: []mtime.Time{mtime.Time(now), mtime.Time(now.Add(time.Hour))}, duration: time.Hour})
	t.Log(b.String())
}
