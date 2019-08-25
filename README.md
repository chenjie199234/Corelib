# Corelib
## nobody
	这是一个简单封装过的http服务器框架。
	使用了golang自带的server。
	用前缀树重写了router使之支持动态url参数和分组（主要参考了fasthttprouter）。
## logger
	这是一个基于本地文件的日志系统，包括logger和collector。
	logger用于打印日志，支持打印到标准输出，文件，以及socket（tcp/unix）。
	collecotr用于收集logger打印到socket的日志。
## mq
	这是一个简单的内存消息队列。
## bitfilter
	这是一个位过滤器。
## buckettree
	这是一颗固定桶个数的一致性树。
## merkletree
	这是一颗简单的默克尔树。
