# codegen

## Description
codegen是一个脚手架工具,用于生成项目初始化代码

## Installation
```
请确认已经设置了Go环境变量GOBIN,并将此环境变量加入到了PATH中
go get -u github.com/chenjie199234/Corelib
cd codegen
go install
```
## 使用
### 1.生成项目:
	终端运行```codegen -d path/to/create/the/project -n "project's name" -g "project belong to group's name"```
### 2.获取依赖:
	终端切换工作目录到项目目录,执行```go mod tidy```
### 3.运行项目:
	linux/mac: 	终端切换工作目录到项目目录,执行```./cmd.sh pb;./cmd.sh run```
	windows: 	终端切换工作目录到项目目录,执行```./cmd.bat pb;./cmd.bat run```
### 4.查看帮助:
	linux/mac: 	终端切换工作目录到项目目录,执行```./cmd.sh```
	windows: 	终端切换工作目录到项目目录,执行```./cmd.bat```

## Features
- [X] Code Generation
