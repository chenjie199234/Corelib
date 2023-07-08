package npm

import (
	"os"
)

const txt = `{
  "private": true,
  "type": "module",
  "dependencies": {
    "axios": "latest",
    "long": "latest",
    "@protobufjs/base64":"latest"
  }
}`

func CreatePathAndFile() {
	file, e := os.OpenFile("./package.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./package.json error: " + e.Error())
	}
	if _, e := file.WriteString(txt); e != nil {
		panic("write ./package.json error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./package.json error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./package.json error: " + e.Error())
	}
}
