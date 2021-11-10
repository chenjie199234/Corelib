package readme

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

const text = `# {{.}}
$$$
{{.}}是一个微服务.
运行cmd脚本可查看使用方法.windows下将./cmd.sh换为cmd.bat
./cmd.sh help 输出帮助信息
./cmd.sh pb 解析proto文件,生成打桩代码
./cmd.sh run 运行该程序
./cmd.sh build 编译该程序,会在根目录下生成一个可执行文件
./cmd.sh new 在该项目中创建一个新的子服务
./cmd.sh kube 增加或者更新kubernetes的配置
$$$


## 环境变量
$$$
CONFIG_TYPE 				配置类型
					0-使用本地配置,路径:./
					1-使用kuberneters的configmap,路径:./kubeconfig
					2-使用远程配置中心配置,路径:./remoteconfig
RUN_ENV 				当前运行环境,如:test,pre,prod
DEPLOY_ENV 				部署环境,如:kube,host
$$$

## 配置文件
$$$
根据环境变量CONFIG_TYPE的不同,配置文件的路径也不同,详情见环境变量CONFIG_TYPE
AppConfig.json该文件配置了该服务需要使用的业务配置,可热更新
SourceConfig.json该文件配置了该服务需要使用的资源配置,不热更新
$$$

## 初始化git
$$$
在项目根目录下执行以下命令初始化git本地仓库
git init
git add .
git commit -m "code generate"
在git远程服务器上创建仓库,然后执行以下命令,将本地仓库与远程仓库关联
git remote add origin path/to/your/remote/repo
git push -u origin main
$$$

## 开发
$$$
切换到开发分支
git checkout -b your/branch/name/recommend/to/use/feature/name
开发完成后先提交到开发分支
git add .
git commit -m "your/commit/to/this/code/change"
将代码推送到git远程服务器,执行下面命令会要求set-upstream,直接复制git输出的命令运行
git push
登陆到git远程服务器前端页面,创建merge,将代码提交到master,等待review和审批(记得勾选删除开发分支选项)
merge完成后在自己的本地仓库执行
git checkout main
git pull
以下为可选步骤,用于删除本地开发分支和本地远程分支
git checkout main
git pull
git branch -D your/branch/name/recommend/to/use/feature/name
git remote prune origin
$$$`

const path = "./"
const name = "README.md"

var tml *template.Template
var file *os.File

func init() {
	var e error
	tml, e = template.New("api").Parse(strings.Replace(text, "$", "`", -1))
	if e != nil {
		panic(fmt.Sprintf("create template error:%s", e))
	}
}
func CreatePathAndFile() {
	var e error
	if e = os.MkdirAll(path, 0755); e != nil {
		panic(fmt.Sprintf("make dir:%s error:%s", path, e))
	}
	file, e = os.OpenFile(path+name, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic(fmt.Sprintf("make file:%s error:%s", path+name, e))
	}
}
func Execute(projectname string) {
	if e := tml.Execute(file, projectname); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
