package monitor

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"
	"time"
)

var heapdata []map[string]string

func Test_Monitor(t *testing.T) {
	heapdata = make([]map[string]string, 0, 1000)
	heap()
	heapdata = nil
	runtime.GC()
	time.Sleep(time.Second * 11)
	WebClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond)*500)
	WebClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond))
	WebServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	WebServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond))
	GrpcClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	GrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*2)
	GrpcServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond)*500)
	GrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*10)
	CrpcClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond)*500)
	CrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*100)
	CrpcServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond)*500)
	CrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*1000)
	testm := getMonitorInfo()
	d, e := json.Marshal(testm)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(string(d))
}
func heap() {
	for i := 0; i < 100000; i++ {
		heapdata = append(heapdata, make(map[string]string, 10))
	}
}
