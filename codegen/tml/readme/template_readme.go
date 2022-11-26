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
./cmd.sh new 在该项目中创建一个新的子服务
./cmd.sh kube 增加或者更新kubernetes的配置
$$$

### 服务端口
$$$
6060                                    MONITOR AND PPROF
8000                                    WEB
9000                                    CRPC
10000                                   GRPC
$$$

## 环境变量
$$$
LOG_LEVEL                               日志等级,debug,info(default),warning,error
LOG_TRACE                               是否开启链路追踪,1-开启,0-关闭(default)
LOG_TARGET                              日志输出目标,std-输出到标准输出,file-输出到文件(可执行文件相同目录),both-两者都输出
GROUP                                   该项目所属的group(k8s的namespace),如果不使用k8s需要手动指定,如果使用k8s,需修改项目根目录的deployment.yaml中的<GROUP>
RUN_ENV                                 当前运行环境,如:test,pre,prod
DEPLOY_ENV                              部署环境,如:ali-kube-shanghai-1,ali-host-hangzhou-1
MONITOR                                 是否开启系统监控采集,0关闭,1开启
CONFIG_TYPE                             配置类型
                                        0-使用本地配置
                                        1-使用远程配置中心config服务
REMOTE_CONFIG_SERVICE_GROUP             当CONFIG_TYPE为1时,配置中心服务的group(k8s的namespace)
REMOTE_CONFIG_SERVICE_HOST              当CONFIG_TYPE为1时,配置中心服务的host地址,[http://https]://[username[:password]@]the.host.name[:port]
REMOTE_CONFIG_SECRET                    当CONFIG_TYPE为1时,配置中心配置的密钥
$$$

## 配置文件
$$$
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
func Execute(ProjectName string) {
	if e := tml.Execute(file, ProjectName); e != nil {
		panic(fmt.Sprintf("write content into file:%s error:%s", path+name, e))
	}
}
