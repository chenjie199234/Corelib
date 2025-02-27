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
6060                                    PPROF
7000                                    RAW TCP OR WEBSOCKET
8000                                    WEB
9000                                    CRPC
10000                                   GRPC
$$$

## 环境变量
$$$
PROJECT                                 该项目所属的项目,[a-z][0-9],第一个字符必须是[a-z]
GROUP                                   该项目所属的项目下的小组,[a-z][0-9],第一个字符必须是[a-z]
RUN_ENV                                 当前运行环境,如:test,pre,prod
DEPLOY_ENV                              部署环境,如:ali-kube-shanghai-1,ali-host-hangzhou-1
TRACE                                   是否开启链路追踪,空-不启用,不空-trace输出方式,[log,oltp,zipkin]
METRIC                                  是否开启系统监控采集,空-不启用,不空-metric输出方式,[log,oltp,prometheus]

CONFIG_TYPE                             配置类型:0-使用本地配置.1-使用admin服务的远程配置中心功能
REMOTE_CONFIG_SECRET                    当CONFIG_TYPE为1时,admin服务中,该服务使用的配置加密密钥,最长31个字符
ADMIN_SERVICE_PROJECT                   当使用admin服务的远程配置中心,服务发现,权限管理功能时,需要设置该环境变量,该变量为admin服务所属的项目,[a-z][0-9],第一个字符必须是[a-z]
ADMIN_SERVICE_GROUP                     当使用admin服务的远程配置中心,服务发现,权限管理功能时,需要设置该环境变量,该变量为admin服务所属的项目下的小组,[a-z][0-9],第一个字符必须是[a-z]
ADMIN_SERVICE_WEB_HOST                  当使用admin服务的远程配置中心,服务发现,权限管理功能时,需要设置该环境变量,该变量为admin服务的host,不带scheme(tls取决于NewSdk时是否传入tls.Config)
ADMIN_SERVICE_WEB_PORT                  当使用admin服务的远程配置中心,服务发现,权限管理功能时,需要设置该环境变量,该变量为admin服务的web端口,默认为80/443(取决于NewSdk时是否使用tls)
ADMIN_SERVICE_CONFIG_ACCESS_KEY         当使用admin服务的远程配置中心功能时,admin服务的授权码
ADMIN_SERVICE_DISCOVER_ACCESS_KEY       当使用admin服务的服务发现功能时,admin服务的授权码
ADMIN_SERVICE_PERMISSION_ACCESS_KEY     当使用admin服务的权限控制功能时,admin服务的授权码

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
