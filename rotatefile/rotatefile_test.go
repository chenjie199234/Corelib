package rotatefile

import (
	"testing"
	"time"
)

func Test_RotateFile(t *testing.T) {
	_, e := NewRotateFile("./log", "", ".log", 1, 0, 1)
	if e != nil {
		panic(e)
	}
	//time.Sleep(10 * time.Second)
	time.Sleep(time.Second)
}
