# Corelib
## web
	web框架,内含proto解析工具,可由proto生成web打桩代码,极大地方便了项目开发
## stream
	tcpsocket,unixsocket长连接框架
## rpc
	基于stream的tcpsocket的rpc框架,内含proto解析工具,可由proto生成rpc打桩代码,极大地方便了项目开发
## codegen
	代码脚手架
## id
	分布式id生成器
## log
	日志
## superd
	守护进程引擎,类似supervisor,fork子进程执行任务,可用于构建cicd平台,或者作为子进程监控器
## discovery
	1.基于stream的tcpsocket的简易注册中心,客户端向所有的discoveryserver监听关注的信息然后将信息在客户端做整合
	2.每个client都会维持于所有discoveryserver的tcp长链接,因此节点变动时能实时更新,节点变动时信息的同步延迟小
