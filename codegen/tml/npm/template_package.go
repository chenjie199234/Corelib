package npm

import (
	"os"
)

const txt = `{
  "private": true,
  "type": "module",
  "dependencies": {
    "axios": "^1.4.0",
    "long": "^5.2.3",
    "@protobufjs/base64":"^1.1.2"
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
