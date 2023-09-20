package ctime

import (
	"encoding/json"
	"testing"
	//"time"
)

type TData struct {
	DD1 Duration            `json:"dd1"`
	DD2 Duration            `json:"dd2"`
	DD3 Duration            `json:"dd3"`
	DD4 Duration            `json:"dd4"`
	DD5 Duration            `json:"dd5"`
	DD6 Duration            `json:"dd6"`
	DD7 map[string]Duration `json:"dd7"`
}

func Test_Mtime(t *testing.T) {
	data := `{"dd1":"0h0m3s8ms9us10ns","dd2":"","dd3":0,"dd4":"0","dd5":"10","dd6":10,"dd7":{"a":"1s","b":"2m"}}`
	d := &TData{}
	e := json.Unmarshal([]byte(data), d)
	if e != nil {
		t.Fatal(e)
	}
	tmp, e := json.Marshal(d)
	if e != nil {
		panic(e)
	}
	t.Log(string(tmp))
}
