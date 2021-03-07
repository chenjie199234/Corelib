package rotatefile

import (
	"testing"
	"time"
)

func Test_RotateFile(t *testing.T) {
	c := &Config{
		Path:        "./log",
		Name:        "",
		Ext:         ".log",
		RotateCap:   1,
		RotateCycle: 1,
		KeepDays:    1,
	}
	_, e := NewRotateFile(c)
	if e != nil {
		panic(e)
	}
	//time.Sleep(10 * time.Second)
	time.Sleep(time.Second)
}
