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
!tsconfig.node.json
!vite.config.ts
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
  <body>
  <div id="app" style="width:100vw;height:100vh;overflow:hidden;background-color:white"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>`

const tsconfig = `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "noImplicitAny": false,
    "noImplicitThis":false,

    /* Bundler mode */
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "preserve",

    /* Linting */
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src/**/*.ts", "src/**/*.d.ts", "src/**/*.tsx", "src/**/*.vue"],
  "references": [{ "path": "./tsconfig.node.json" }]
}`

const tsconfignode = `{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}`

const viteconfig = `import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
  }
})`

const pkg = `{
  "name": "{{.}}",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "latest"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "latest",
    "typescript": "latest",
    "vite": "latest",
    "vue-tsc": "latest"
  }
}`

const main = `import { createApp } from 'vue'
import main from './main.vue'

createApp(main).mount('#app')`

const app = `<script setup lang="ts">
</script>

<template>
Hello World
</template>`

const viteenv = `/// <reference types="vite/client" />`

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
	tsconfigfile, e := os.OpenFile("./html/tsconfig.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/tsconfig.json error: " + e.Error())
	}
	if _, e := tsconfigfile.WriteString(tsconfig); e != nil {
		panic("write ./html/tsconfig.json error: " + e.Error())
	}
	if e := tsconfigfile.Sync(); e != nil {
		panic("sync ./html/tsconfig.json error: " + e.Error())
	}
	if e := tsconfigfile.Close(); e != nil {
		panic("close ./html/tsconfig.json error: " + e.Error())
	}
	//./html/tsconfig.node.json
	tsconfignodefile, e := os.OpenFile("./html/tsconfig.node.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/tsconfig.node.json error: " + e.Error())
	}
	if _, e := tsconfignodefile.WriteString(tsconfignode); e != nil {
		panic("write ./html/tsconfig.node.json error: " + e.Error())
	}
	if e := tsconfignodefile.Sync(); e != nil {
		panic("sync ./html/tsconfig.node.json error: " + e.Error())
	}
	if e := tsconfignodefile.Close(); e != nil {
		panic("close ./html/tsconfig.node.json error: " + e.Error())
	}
	//./html/vite.config.ts
	viteconfigfile, e := os.OpenFile("./html/vite.config.ts", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/vite.config.ts error: " + e.Error())
	}
	if _, e := viteconfigfile.WriteString(viteconfig); e != nil {
		panic("write ./html/vite.config.ts error: " + e.Error())
	}
	if e := viteconfigfile.Sync(); e != nil {
		panic("sync ./html/vite.config.ts error: " + e.Error())
	}
	if e := viteconfigfile.Close(); e != nil {
		panic("close ./html/vite.config.ts error: " + e.Error())
	}
	//./html/src/main.vue
	appfile, e := os.OpenFile("./html/src/main.vue", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/src/main.vue error: " + e.Error())
	}
	if _, e := appfile.WriteString(app); e != nil {
		panic("write ./html/src/main.vue error: " + e.Error())
	}
	if e := appfile.Sync(); e != nil {
		panic("sync ./html/src/main.vue error: " + e.Error())
	}
	if e := appfile.Close(); e != nil {
		panic("close ./html/src/main.vue error: " + e.Error())
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
	//./html/src/vite-env.d.ts
	viteenvfile, e := os.OpenFile("./html/src/vite-env.d.ts", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./html/src/vite-env.d.ts error: " + e.Error())
	}
	if _, e := viteenvfile.WriteString(viteenv); e != nil {
		panic("write ./html/src/vite-env.d.ts error: " + e.Error())
	}
	if e := viteenvfile.Sync(); e != nil {
		panic("sync ./html/src/vite-env.d.ts error: " + e.Error())
	}
	if e := viteenvfile.Close(); e != nil {
		panic("close ./html/src/vite-env.d.ts error: " + e.Error())
	}
}