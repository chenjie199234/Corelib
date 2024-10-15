# Corelib
![image](Corelib.jpg)
# Use
	1.install golang(1.23+)
	2.install git
	3.install protoc
	4.install protoc-gen-go
	5.[download the codegen in release] or [clone this lib and go into the codegen dir and install it by go install]
	6.make sure the codegen's path is in the system's or user's PATH env
	7.codegen -d path -n projectname -p packagename
	(without -d or -d is empty the default:current dir will be used)
	(projectname format,character must in [a-z][0-9],first character must in [a-z])
	(packagename must end with projectname,default:projectname will be used as packagename,e.g. github.com/path_to_the_repo/projectname)
# Description
## codegen
	generate the base code,if you want to use this lib,this is the start
## web
	Web framework,this contains protobuf plugin to generate code from proto file
## cgrpc
	grpc framework,this contains protobuf plugin to generate code from proto file
## crpc
	a new rpc framework(20-30% fast then grpc),this contains protobuf plugin to generate code from proto file
## pbex
    protobuf extension for message validate,this is used in the web/crpc/cgrpc framework
## stream
	stream connection framework,use websocket's frame format,so this can support rawtcp and websocket at the same time on same port
## ws
    a simple ws handshake and message recv and send framework
## container
	some useful struct
## monitor
    Promethus support
## mysql
    mysql driver wraper,add trace support and read write seprate
## mongo
    mongodb driver wraper,add trace support
## redis
    redis driver wraper,add trace support and some useful function(bloom filter,rate limiter,message queue,verify code check)
## email
    simple email client used to send email
## superd
	fork child process to exec some other program,like supervisor
