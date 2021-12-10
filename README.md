# Corelib
![image](Corelib.jpg)
# Use
	1.安装golang(1.17+)
	2.安装git
	3.安装protoc
	4.安装protoc-gen-go
	5.下载release中的二进制文件codegen
	6.执行codegen -d path -n projectname -g groupname(k8s namespace)
	(没有-d则默认在当前目录创建)
	(projectname格式,[a-z][A-Z][0-9][_],首字母必须是[a-z][A-Z])
	(groupname格式,[a-z][A-Z][0-9][-_|()[]{}<>],首字母必须是[a-z][A-Z])
## web
	web框架,内含proto解析工具,可由proto生成web打桩代码
## cgrpc
	grpc框架,二次封装,内含proto解析工具,可由proto生成web打桩代码
## stream
	tcp长连接框架
## crpc
	因grpc性能较差,所以重新造了个rpc轮子,性能相较grpc有15%+的提升.基于stream的rpc框架,内含proto解析工具,可由proto生成crpc打桩代码
## codegen
	代码脚手架
## error
	错误格式
## nacos
	nacos远程配置中心的封装
## id
	分布式id生成器
## log
	日志
## trace
	链路追踪
## bufpool
	共享缓存,用于缓存复用,减少gc
## container
	常用的一些数据结构
## superd
	守护进程引擎,类似supervisor,fork子进程执行任务,可用于构建cicd平台,或者作为子进程监控器
## config
	配置中心
	https://github.com/chenjie199234/Config
## discovery(不建议使用,目前该框架生成的代码直接使用了kuberneters的dns进行服务发现)
	服务注册和发现
	https://github.com/chenjie199234/Discovery
