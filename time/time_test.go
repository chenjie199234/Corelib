package time

import (
	"encoding/json"
	"testing"
	"time"
)

type TData struct {
	TT Time     `json:"tt"`
	DD Duration `json:"dd"`
}

func Test_Mtime(t *testing.T) {
	data := `{"tt":"2021-01-02 05:06:07.123789682 +00","dd":"1h10m3s8ms9us10ns"}`
	d := &TData{}
	e := json.Unmarshal([]byte(data), d)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(time.Time(d.TT).Nanosecond())
	t.Log(int64(d.DD))
	data = `{"tt":0,"dd":10}`
	d = &TData{}
	e = json.Unmarshal([]byte(data), d)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(time.Time(d.TT).Nanosecond())
	t.Log(int64(d.DD))
}
