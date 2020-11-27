package memory

import (
	"fmt"
	"time"

	"testing"
)

func Test_Memory(t *testing.T) {
	for {
		time.Sleep(time.Second)
		fmt.Printf("%+v\n", GetUse())
	}
}
