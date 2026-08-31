# codegen

## Description
	codegen是一个脚手架工具,用于生成项目初始化代码

## Installation
	请确认已经设置了Go环境变量GOBIN,并将此环境变量加入到了PATH中
	go get -u github.com/chenjie199234/Corelib
	cd codegen
	go install

## 使用
### 0.查询版本:
    终端运行 codegen -v
### 1.生成项目:
	终端运行 codegen -n "project name" [-p "package name,must be app name or end with app name"]
### 2.查看帮助:
	linux/mac: 	项目内: ./cmd.sh
	windows: 	项目内: ./cmd.bat
### 3.解析proto文件生成桩文件:
	linux/max: 	项目内: ./cmd.sh pb
	windows: 	项目内: ./cmd.bat pb
### 4.创建子服务
	linux/mac: 	项目内: ./cmd.sh sub "sub service name"
	windows: 	项目内: ./cmd.bat sub "sub service name"
### 5.更新kuberneters配置
	linux/mac: 	项目内: ./cmd.sh kube
	windows: 	项目内: ./cmd.bat kube
### 6.创建新的web项目
    linux/max:  项目内: ./cmd.sh html
    windows:    项目内: ./cmd.bat html

## Features
- [X] Code Generation
