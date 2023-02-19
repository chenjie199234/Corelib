package api

import (
	"os"
)

const txt = `package api

// Don't delete!
// This file is a placeholder for the package name!`

func CreatePathAndFile() {
	if e := os.MkdirAll("./api/", 0755); e != nil {
		panic("mkdir ./api/ error: " + e.Error())
	}
	file, e := os.OpenFile("./api/api.go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./api/api.go error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./api/api.go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./api/api.go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./api/api.go error: " + e.Error())
	}
}
