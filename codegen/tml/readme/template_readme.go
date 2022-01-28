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
./cmd.sh pb 解析proto文件,生成桩代码
./cmd.sh run 运行该程序
./cmd.sh build 编译该程序,会在根目录下生成一个可执行文件
./cmd.sh new 在该项目中创建一个新的子服务
./cmd.sh kube 增加或者更新kubernetes的配置
$$$

### 服务端口
$$$
8000                                    Web
9000                                    CRPC
10000                                   GRPC
$$$

## 环境变量
$$$
GROUP                                   该项目所属的分组(k8s的namespace名字),如果不使用k8s需要手动指定,如果使用k8s无需手动指定,请查看项目根目录的deployment.yaml
MONITOR                                 是否开启系统监控采集,0关闭,1开启
CONFIG_TYPE 				配置类型
					0-使用本地配置,路径:./
					1-使用kuberneters的configmap,路径:./kubeconfig
					2-使用远程配置中心配置,路径:./remoteconfig
RUN_ENV 				当前运行环境,如:test,pre,prod
DEPLOY_ENV 				部署环境,如:kube,host
REMOTE_CONFIG_USERNAME			当CONFIG_TYPE为2时,设置远程配置中心的数据库用户名(只读账号)
REMOTE_CONFIG_PASSWORD			当CONFIG_TYPE为2时,设置远程配置中心的数据库密码(只读账号)
REMOTE_CONFIG_REPLICASET		当CONFIG_TYPE为2时,如果远程配置中心的数据库是以副本集模式部署的,需要设置replicaset,如果是分片模式部署的,不需要设置
REMOTE_CONFIG_ADDRS			当CONFIG_TYPE为2时,设置远程配置中心的数据库地址(ip:port),多个地址使用逗号分隔
$$$

## 配置文件
$$$
根据环境变量CONFIG_TYPE的不同,配置文件的路径也不同,详情见环境变量CONFIG_TYPE
AppConfig.json该文件配置了该服务需要使用的业务配置,可热更新
SourceConfig.json该文件配置了该服务需要使用的资源配置,不热更新
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
