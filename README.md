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
	1.基于stream的tcpsocket的简易注册中心,client会全量备份每一个discoveryserver的全量注册信息并实时更新,然后在client端进行数据整合.
	2.有n个discoveryserver,集群中有y个client时,每一个client都会维护n*y的注册信息并实时更新,当集群非常大并且更新频繁时,会对client有比较大的影响
	3.但是当集群不大时,集群更新的影响会变小,同时由于是实时更新,所以节点变动时信息的同步延迟变小
