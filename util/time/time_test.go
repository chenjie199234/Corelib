package time

import (
	"encoding/json"
	"testing"
	//"time"
)

type TData struct {
	DD Duration `json:"dd"`
}

func Test_Mtime(t *testing.T) {
	data := `{"dd":"1h10m3s8ms9us10ns"}`
	d := &TData{}
	e := json.Unmarshal([]byte(data), d)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(d.DD)
	tmp, e := json.Marshal(d)
	if e != nil {
		panic(e)
	}
	t.Log(string(tmp))
}
