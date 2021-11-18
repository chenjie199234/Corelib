package log

import (
	"errors"
	"testing"
	"time"

	cerror "github.com/chenjie199234/Corelib/error"
	ctime "github.com/chenjie199234/Corelib/util/time"
)

type A struct {
	Name string
}

func Test_Log(t *testing.T) {
	du := time.Second
	cdu := ctime.Duration(du)
	now := time.Now()
	e := errors.New("e")
	ee := cerror.MakeError(1, 500, "msg")
	a := A{Name: "name"}
	Error(nil, int(1), []int{1}, uint(1), []uint{1}, int8(1), []int8{1}, int16(1), []int16{1}, uint16(1), []uint16{1}, int32(1), []int32{1}, uint32(1), []uint32{1}, int64(1), []int64{1}, uint64(1), []uint64{1}, float32(1.1), []float32{1.1}, float64(1.2), []float64{1.2}, byte('g'), []byte("lmn"), [][]byte{[]byte("abc"), []byte("xyz")}, "lmn", []string{"abc", "xyz"}, du, &du, []time.Duration{du}, []*time.Duration{nil, &du}, cdu, &cdu, []ctime.Duration{cdu}, []*ctime.Duration{nil, &cdu}, now, &now, []time.Time{now}, []*time.Time{nil, &now}, e, ee, []error{e, ee, nil}, a, &a, []A{a}, []*A{&a})
}
