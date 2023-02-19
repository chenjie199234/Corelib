package sub

import (
	"os"
)

const txt = `package model`

func CreatePathAndFile(sname string) {
	if e := os.MkdirAll("./model/", 0755); e != nil {
		panic("mkdir ./model/ error: " + e.Error())
	}
	file, e := os.OpenFile("./model/"+sname+".go", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./model/" + sname + ".go error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./model/" + sname + ".go error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./model/" + sname + ".go error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./model/" + sname + ".go error: " + e.Error())
	}
}
