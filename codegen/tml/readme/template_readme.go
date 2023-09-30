package readme

import (
	"os"
	"strings"
	"text/template"
)

const txt = `# {{.}}
$$$
{{.}}是一个微服务.
运行cmd脚本可查看使用方法.windows下将./cmd.sh换为cmd.bat
./cmd.sh help 输出帮助信息
./cmd.sh pb 解析proto文件,生成桩代码
./cmd.sh sub 在该项目中创建一个新的子服务
./cmd.sh kube 新建kubernetes的配置
./cmd.sh html 新建前端html代码模版
$$$

## 服务端口
$$$
6060                                    MONITOR AND PPROF
8000                                    WEB
9000                                    CRPC
10000                                   GRPC
$$$

## 环境变量
$$$
LOG_LEVEL                               日志等级,debug,info(default),warn,error
LOG_TRACE                               是否开启链路追踪,1-开启,0-关闭(default)
LOG_TARGET                              日志输出目标,std-输出到标准输出,file-输出到文件(当前工作目录的./log/目录下)
PROJECT                                 该项目所属的项目,[a-z][0-9],第一个字符必须是[a-z]
GROUP                                   该项目所属的项目组,[a-z][0-9],第一个字符必须是[a-z]
RUN_ENV                                 当前运行环境,如:test,pre,prod
DEPLOY_ENV                              部署环境,如:ali-kube-shanghai-1,ali-host-hangzhou-1
MONITOR                                 是否开启系统监控采集,0关闭,1开启

CONFIG_TYPE                             配置类型:0-使用本地配置.1-使用admin服务的远程配置中心功能
REMOTE_CONFIG_SERVICE_PROJECT           当CONFIG_TYPE为1时,admin服务所属的项目,[a-z][0-9],第一个字符必须是[a-z]
REMOTE_CONFIG_SERVICE_GROUP             当CONFIG_TYPE为1时,admin服务所属的项目组,[a-z][0-9],第一个字符必须是[a-z]
REMOTE_CONFIG_SERVICE_WEB_HOST          当CONFIG_TYPE为1时,admin服务的host,不带scheme(是否启用tls取决于NewConfigSdk时是否传入tls.Config)
REMOTE_CONFIG_SERVICE_WEB_PORT          当CONFIG_TYPE为1时,admin服务的web端口,默认为80/443(取决于NewConfigSdk时是否使用tls)
REMOTE_CONFIG_SECRET                    当CONFIG_TYPE为1时,远程配置的密钥

ADMIN_DISCOVER_SERVICE_PROJECT          当使用admin的服务发现功能时,admin服务所属的项目,[a-z][0-9],第一个字符必须是[a-z]
ADMIN_DISCOVER_SERVICE_GROUP            当使用admin的服务发现功能时,admin服务所属的项目组,[a-z][0-9],第一个字符必须是[a-z]
ADMIN_DISCOVER_SERVICE_WEB_HOST         当使用admin的服务发现功能时,admin服务的host,不带scheme(是否启用tls取决于NewConfigSdk时是否传入tls.Config)
ADMIN_DISCOVER_SERVICE_WEB_PORT         当使用admin的服务发现功能时,admin服务的web端口,默认为80/443(取决于NewConfigSdk时是否使用tls)

PERMISSION_SERVICE_PROJECT              当使用admin的权限控制功能时,admin服务所属的项目,[a-z][0-9],第一个字符必须是[a-z]
PERMISSION_SERVICE_GROUP                当使用admin的权限控制功能时,admin服务所属的项目组,[a-z][0-9],第一个字符必须是[a-z]
PERMISSION_SERVICE_WEB_HOST             当使用admin的权限控制功能时,admin服务的host,不带scheme(是否启用tls取决于NewConfigSdk时是否传入tls.Config)
PERMISSION_SERVICE_WEB_PORT             当使用admin的权限控制功能时,admin服务的web端口,默认为80/443(取决于NewPermissionSdk时是否使用tls)

$$$

## 配置文件
$$$
AppConfig.json该文件配置了该服务需要使用的业务配置,可热更新
SourceConfig.json该文件配置了该服务需要使用的资源配置,不热更新
$$$`

func CreatePathAndFile(projectname string) {
	readmetemplate, e := template.New("./README.md").Parse(strings.Replace(txt, "$", "`", -1))
	if e != nil {
		panic("parse ./README.md template error: " + e.Error())
	}
	file, e := os.OpenFile("./README.md", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if e != nil {
		panic("open ./README.md error: " + e.Error())
	}
	if e := readmetemplate.Execute(file, projectname); e != nil {
		panic("write ./README.md error: " + e.Error())
	}
	if e := file.Sync(); e != nil {
		panic("sync ./README.md error: " + e.Error())
	}
	if e := file.Close(); e != nil {
		panic("close ./README.md error: " + e.Error())
	}
}
