# codegen

## Description
	codegen是一个脚手架工具,用于生成项目初始化代码

## Installation
	请确认已经设置了Go环境变量GOBIN,并将此环境变量加入到了PATH中
	go get -u github.com/chenjie199234/Corelib
	cd codegen
	go install

## 使用
### 1.生成项目:
	终端运行 codegen -d path/to/create/the/project(default is current work dir) -n "project's name"
### 2.查看帮助:
	linux/mac: 	终端切换工作目录到项目目录,执行 ./cmd.sh
	windows: 	终端切换工作目录到项目目录,执行 ./cmd.bat
### 3.解析proto文件生成桩文件:
	linux/max: 	终端切换工作目录到项目目录,执行 ./cmd.sh pb
	windows: 	终端切换工作目录到项目目录,执行 ./cmd.bat pb
### 4.创建子服务
	linux/mac: 	终端切换工作目录到项目目录,执行 ./cmd.sh sub "sub service name"
	windows: 	终端切换工作目录到项目目录,执行 ./cmd.bat sub "sub service name"
### 5.更新kuberneters配置
	linux/mac: 	终端切换工作目录到项目目录,执行 ./cmd.sh kube
	windows: 	终端切换工作目录到项目目录,执行 ./cmd.bat kube

## Features
- [X] Code Generation
