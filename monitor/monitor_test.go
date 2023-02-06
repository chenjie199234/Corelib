package monitor

import (
	"encoding/json"
	"errors"
	"math/rand"
	"runtime"
	"sort"
	"testing"
	"time"
)

var heapdata []map[string]string

func Test_Monitor(t *testing.T) {
	heapdata = make([]map[string]string, 0, 1000)
	heap()
	heapdata = nil
	runtime.GC()
	WebClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	WebClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond))
	WebClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*2)

	WebServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	WebServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond))
	WebServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*2)

	GrpcClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	GrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*2)
	GrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*5)

	GrpcServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	GrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*10)
	GrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*50)

	CrpcClientMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	CrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*100)
	CrpcClientMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*500)

	CrpcServerMonitor("test.abc", "POST", "/api", nil, uint64(time.Microsecond*500))
	CrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*1000)
	CrpcServerMonitor("test.abc", "POST", "/api", errors.New("test error"), uint64(time.Millisecond)*5000)
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
func Test_GetT(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	data := [5001]uint32{}
	maxtimewaste := uint64(time.Second * 7)
	normalcount := uint32(20)
	slowcount := uint32(10)
	tmp := make([]int, 0, normalcount)
	for i := 0; i < int(normalcount); i++ {
		r := rand.Intn(5000)
		tmp = append(tmp, r+1)
		data[r]++
	}
	data[5000] = 1 + slowcount
	t50, t90, t99 := getT(&data, maxtimewaste, normalcount+slowcount+1)
	sort.Ints(tmp)
	t.Log(tmp, " 50:", t50, " 90:", t90, " 99:", t99)
}
