package html

import (
	"os"
	"text/template"
)

const git = `*
!.gitignore
!index.html
!package.json
!tsconfig.json
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
  <body id="app" style="margin:0px;padding:0px;width:100vw;height:100vh;overflow:hidden">
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>`

const pkg = `{
  "name": "{{.}}",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "devDependencies": {
    "vite-tsconfig-paths": "latest",
    "typescript": "latest",
    "vite": "latest"
  },
  "dependencies": {
  }
}`

const tsc = `{
  "compilerOptions": {
    "paths":{
      "@api/*":["../api/*"]
    },
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,

    /* Bundler mode */
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "moduleDetection": "force",
    "isolatedModules": true,
    "noEmit": true,
    "noEmitOnError": true,

    /* Linting */
    "strict": true,
    "alwaysStrict": true,
    "strictNullChecks": true,
    "noImplicitAny": true,
    "noImplicitThis": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
	"noUncheckedSideEffectImports": true
  },
  "include": ["src"]
}`
const vitec = `import { defineConfig } from "vite"
import tsconfigPaths from "vite-tsconfig-paths"

export default defineConfig({
  plugins: [tsconfigPaths()],
})`

const main = `document.querySelector<HTMLDivElement>('#app')!.innerHTML="hello world"`

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
	//./html/tsconfig.json
	tscfile, e := os.OpenFile("./html/tsconfig.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/tsconfig.json error: " + e.Error())
	}
	if _, e := tscfile.WriteString(tsc); e != nil {
		panic("write ./html/tsconfig.json error: " + e.Error())
	}
	if e := tscfile.Sync(); e != nil {
		panic("sync ./html/tsconfig.json error: " + e.Error())
	}
	if e := tscfile.Close(); e != nil {
		panic("close ./html/tsconfig.json error: " + e.Error())
	}
	//./html/vite.config.ts
	vitecfile, e := os.OpenFile("./html/vite.config.ts", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/vite.config.ts error: " + e.Error())
	}
	if _, e := vitecfile.WriteString(vitec); e != nil {
		panic("write ./html/vite.config.ts error: " + e.Error())
	}
	if e := vitecfile.Sync(); e != nil {
		panic("sync ./html/vite.config.ts error: " + e.Error())
	}
	if e := vitecfile.Close(); e != nil {
		panic("close ./html/vite.config.ts error: " + e.Error())
	}
	//./html/src/main.ts
	mainfile, e := os.OpenFile("./html/src/main.ts", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/src/main.ts error: " + e.Error())
	}
	if _, e := mainfile.WriteString(main); e != nil {
		panic("write ./html/src/main.ts error: " + e.Error())
	}
	if e := mainfile.Sync(); e != nil {
		panic("sync ./html/src/main.ts error: " + e.Error())
	}
	if e := mainfile.Close(); e != nil {
		panic("close ./html/src/main.ts error: " + e.Error())
	}
}
