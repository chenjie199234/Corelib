package trace

import (
	"fmt"
	"math/rand"
	"time"
)

var r *rand.Rand

func init() {
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func Trace(origin, appname string) string {
	if origin == "" {
		now := time.Now()
		return fmt.Sprintf("%s.%d|%d|%s", now.Format("2006-01-02 15:04:05"), now.Nanosecond(), r.Int63(), appname)
	}
	return fmt.Sprintf("%s|%s", origin, appname)
}
