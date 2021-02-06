package bufpool

import (
	"testing"
	"time"

	"github.com/chenjie199234/Corelib/mtime"
)

type A struct {
	Name     string
	now      []mtime.MTime
	duration time.Duration
}

func Test_Bufpool(t *testing.T) {
	b := GetBuffer()
	now := time.Now()
	b.Append(&A{Name: "name", now: []mtime.MTime{mtime.MTime(now), mtime.MTime(now.Add(time.Hour))}, duration: time.Hour})
	t.Log(b.String())
}
