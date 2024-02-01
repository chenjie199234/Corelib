package html

import (
	"os"
	"text/template"
)

const git = `*
!.gitignore
!index.html
!package.json
!/src/
!/src/*
!/src/**/
!/src/**/*`

const index = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{.}}</title>
  </head>
  <body style="margin:0px;padding:0px;border:0px none white">
    <div id="app" style="width:100vw;height:100vh;overflow:hidden;background-color:white"></div>
    <script type="module" src="/src/main.js"></script>
  </body>
</html>`

const pkg = `{
  "name": "{{.}}",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "devDependencies": {
    "vite": "latest"
  },
  "dependencies": {
  }
}`

const main = `document.querySelector('#app').innerHTML="hello world"`

func CreatePathAndFile(projectname string) {
	var e error
	if e = os.MkdirAll("./html/src", 0755); e != nil {
		panic("mkdir ./html/src/ error: " + e.Error())
	}
	//./html/.gitignore
	gitfile, e := os.OpenFile("./html/.gitignore", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/.gitignore error: " + e.Error())
	}
	if _, e := gitfile.WriteString(git); e != nil {
		panic("write ./html/.gitignore error: " + e.Error())
	}
	if e := gitfile.Sync(); e != nil {
		panic("sync ./html/.gitignore error: " + e.Error())
	}
	if e := gitfile.Close(); e != nil {
		panic("close ./html/.gitignore error: " + e.Error())
	}
	//./html/index.html
	indextemplate, e := template.New("./html/index.html").Parse(index)
	if e != nil {
		panic("parse ./html/index.html template error: " + e.Error())
	}
	indexfile, e := os.OpenFile("./html/index.html", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/index.html error: " + e.Error())
	}
	if e := indextemplate.Execute(indexfile, projectname); e != nil {
		panic("write ./html/index.html error: " + e.Error())
	}
	if e := indexfile.Sync(); e != nil {
		panic("sync ./html/index.html error: " + e.Error())
	}
	if e := indexfile.Close(); e != nil {
		panic("close ./html/index.html error: " + e.Error())
	}
	//./html/package.json
	pkgtemplate, e := template.New("./html/package.json").Parse(pkg)
	if e != nil {
		panic("parse ./html/package.json template error: " + e.Error())
	}
	pkgfile, e := os.OpenFile("./html/package.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/package.json error: " + e.Error())
	}
	if e := pkgtemplate.Execute(pkgfile, projectname); e != nil {
		panic("write ./html/package.json error: " + e.Error())
	}
	if e := pkgfile.Sync(); e != nil {
		panic("sync ./html/package.json error: " + e.Error())
	}
	if e := pkgfile.Close(); e != nil {
		panic("close ./html/package.json error: " + e.Error())
	}
	//./html/src/main.js
	mainfile, e := os.OpenFile("./html/src/main.js", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/src/main.js error: " + e.Error())
	}
	if _, e := mainfile.WriteString(main); e != nil {
		panic("write ./html/src/main.js error: " + e.Error())
	}
	if e := mainfile.Sync(); e != nil {
		panic("sync ./html/src/main.js error: " + e.Error())
	}
	if e := mainfile.Close(); e != nil {
		panic("close ./html/src/main.js error: " + e.Error())
	}
}
