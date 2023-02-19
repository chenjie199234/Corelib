package util

import (
	"os"
)

const txt = `package util`

func CreatePathAndFile() {
	if e := os.MkdirAll("./util/", 0755); e != nil {
		panic("mkdir ./util/ error: " + e.Error())
	}
	file, e := os.OpenFile("./util/util.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./util/util.go error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./util/util.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./util/util.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./util/util.go error: " + e.Error())
	}
}
