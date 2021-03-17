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
./cmd.sh pb 解析proto文件,生成打桩代码
./cmd.sh run 运行该程序
./cmd.sh build 编译该程序,会在根目录下生成一个可执行文件
./cmd.sh new 在该项目中创建一个新的子服务
./cmd.sh help 输出帮助信息
./cmd.sh kubernetes 增加或者更新kubernetes的配置
$$$


## 环境变量
$$$
DISCOVERY_SERVER_GROUP 			指定注册中心服务器所属的group名字
DISCOVERY_SERVER_NAME 			指定注册中心服务器自身的名字
DISCOVERY_SERVER_PORT  			指定注册中心服务器监听的端口
DISCOVERY_SERVER_VERIFY_DATA 		连接注册中心服务器使用的校验数据
OLD_RPC_VERIFY_DATA 			自身启动rpc服务时,当进行校验数据更新时才使用
RPC_VERIFY_DATA 			自身启动rpc服务时,当前正在使用的最新的校验数据
OLD_PPROF_VERIFY_DATA 			web服务器使用pprof时才有效,当进行校验数据更新时才使用
PPROF_VERIFY_DATA 			web服务器使用pprof时才有效,当前正在使用的最新的校验数据
REMOTE_CONFIG 				是否使用远程配置中心的配置[true/false]
RUN_ENV 				当前运行环境,如:test,pre,prod
DEPLOY_ENV 				部署环境,如:k8s,host
$$$

## 配置文件
$$$
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
		panic(fmt.Sprintf("create template for %s error:%s", path+name, e))
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
		panic(fmt.Sprintf("write content into file:%s from template error:%s", path+name, e))
	}
}
