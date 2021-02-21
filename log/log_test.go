package log

import (
	"os"
	"testing"
)

type A struct {
	Name string
	Age  int64
}

func Test_Mlog(t *testing.T) {
	Debug("testdebug", "a", 1)
	Info("testinfo", []int{1, 2}, []string{"a", "b"})
	Warning("testwarning", true, []bool{false, true})
	testdata := make(map[int]*A)
	testdata[1] = &A{Name: "1", Age: 1}
	testdata[2] = &A{Name: "2", Age: 2}
	Error("testerror", testdata, &A{Name: "name", Age: 18})
	Close()
	if e := os.RemoveAll("./log"); e != nil {
		panic(e)
	}
}
