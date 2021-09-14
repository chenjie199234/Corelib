# Corelib
![image](Corelib.jpg)
## web
	web框架,内含proto解析工具,可由proto生成web打桩代码
## stream
	tcp长连接框架
## rpc
	基于stream的rpc框架,内含proto解析工具,可由proto生成rpc打桩代码
## codegen
	代码脚手架
## id
	分布式id生成器
## log
	日志
## superd
	守护进程引擎,类似supervisor,fork子进程执行任务,可用于构建cicd平台,或者作为子进程监控器
## config
	配置中心
	https://github.com/chenjie199234/Config
## discovery(不建议使用,目前该框架生成的代码直接使用了kuberneters的dns进行服务发现)
	服务注册和发现
	https://github.com/chenjie199234/Discovery
