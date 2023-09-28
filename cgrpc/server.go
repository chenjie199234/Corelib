package cgrpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	cmetadata "github.com/chenjie199234/Corelib/metadata"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/ctime"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	//the default timeout for every rpc call,<=0 means no timeout
	//if specific path's timeout setted by UpdateHandlerTimeout,this specific path will ignore the GlobalTimeout
	//the client's deadline will also effect the rpc call's final deadline
	GlobalTimeout ctime.Duration `json:"global_timeout"`
	//time for connection establish(include dial time,handshake time and verify time)
	//default 3s
	ConnectTimeout ctime.Duration `json:"connect_timeout"`
	//connection will be closed if it is not actived after this time,<=0 means no idletimeout
	IdleTimeout ctime.Duration `json:"idle_timeout"`
	//min 1s,default 5s,3 probe missing means disconnect
	HeartProbe ctime.Duration `json:"heart_probe"`
	//min 64k,default 64M
	MaxMsgLen uint32 `json:"max_msg_len"`
}

func (c *ServerConfig) validate() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = ctime.Duration(3 * time.Second)
	}
	if c.IdleTimeout < 0 {
		c.IdleTimeout = 0
	}
	if c.HeartProbe <= 0 {
		c.HeartProbe = ctime.Duration(time.Second * 5)
	} else if c.HeartProbe.StdDuration() < time.Second {
		c.HeartProbe = ctime.Duration(time.Second)
	}
	if c.MaxMsgLen == 0 {
		c.MaxMsgLen = 1024 * 1024 * 64
	} else if c.MaxMsgLen < 65536 {
		c.MaxMsgLen = 65536
	}
}

type CGrpcServer struct {
	c              *ServerConfig
	self           string
	global         []OutsideHandler
	ctxpool        *sync.Pool
	server         *grpc.Server
	clientnum      int32
	services       map[string]*grpc.ServiceDesc
	handlerTimeout map[string]time.Duration
	totalreqnum    int64
}

// if tlsc is not nil,the tls will be actived
func NewCGrpcServer(c *ServerConfig, selfproject, selfgroup, selfapp string, tlsc *tls.Config) (*CGrpcServer, error) {
	if tlsc != nil {
		if len(tlsc.Certificates) == 0 && tlsc.GetCertificate == nil && tlsc.GetConfigForClient == nil {
			return nil, errors.New("[cgrpc.NewCGrpcServer] tls certificate setting missing")
		}
		tlsc = tlsc.Clone()
	}
	//pre check
	selffullname, e := name.MakeFullName(selfproject, selfgroup, selfapp)
	if e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CGrpcServer{
		c:              c,
		self:           selffullname,
		global:         make([]OutsideHandler, 0),
		ctxpool:        &sync.Pool{},
		services:       make(map[string]*grpc.ServiceDesc),
		handlerTimeout: make(map[string]time.Duration),
	}
	opts := make([]grpc.ServerOption, 0, 10)
	opts = append(opts, grpc.RecvBufferPool(pool.GetPool()))
	opts = append(opts, grpc.MaxRecvMsgSize(int(c.MaxMsgLen)))
	opts = append(opts, grpc.StatsHandler(serverinstance))
	opts = append(opts, grpc.UnknownServiceHandler(func(_ interface{}, stream grpc.ServerStream) error {
		ctx := stream.Context()
		rpcinfo := ctx.Value(serverrpckey{}).(*stats.RPCTagInfo)
		gmd, ok := gmetadata.FromIncomingContext(ctx)
		peerip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				peerip = strings.TrimSpace(realip[0])
			}
		}
		if peerip == "" {
			conninfo := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
			remoteaddr := conninfo.RemoteAddr.String()
			peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}
		log.Error(nil, "[cgrpc.server] path doesn't exist", log.String("cip", peerip), log.String("path", rpcinfo.FullMethodName))
		return cerror.ErrNoapi
	}))
	opts = append(opts, grpc.ConnectionTimeout(c.ConnectTimeout.StdDuration()))
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle: c.IdleTimeout.StdDuration(),
		Time:              c.HeartProbe.StdDuration(),
		Timeout:           c.HeartProbe.StdDuration() * 3,
	}))
	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: time.Millisecond * 999, PermitWithoutStream: true}))
	if tlsc != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsc)))
	}
	serverinstance.server = grpc.NewServer(opts...)
	return serverinstance, nil
}

var ErrServerClosed = errors.New("[cgrpc.server] closed")

func (s *CGrpcServer) StartCGrpcServer(listenaddr string) error {
	l, e := net.Listen("tcp", listenaddr)
	if e != nil {
		return errors.New("[cgrpc.server] listen tcp addr: " + listenaddr + " " + e.Error())
	}
	for _, service := range s.services {
		s.server.RegisterService(service, nil)
	}
	if e := s.server.Serve(l); e != nil {
		if e == grpc.ErrServerStopped {
			return ErrServerClosed
		}
		return e
	}
	return nil
}
func (this *CGrpcServer) GetClientNum() int32 {
	return this.clientnum
}
func (this *CGrpcServer) GetReqNum() int64 {
	return this.totalreqnum
}

// force - false graceful,wait all requests finish,true - not graceful,close all connections immediately
func (s *CGrpcServer) StopCGrpcServer(force bool) {
	if force {
		s.server.Stop()
	} else {
		s.server.GracefulStop()
	}
}

// first key path,second key method,value timeout(if timeout <= 0 means no timeout)
func (this *CGrpcServer) UpdateHandlerTimeout(timeout map[string]map[string]ctime.Duration) {
	tmp := make(map[string]time.Duration)
	for path := range timeout {
		for method, to := range timeout[path] {
			if method != "GRPC" {
				continue
			}
			if path == "" {
				continue
			}
			if path[0] != '/' {
				path = "/" + path
			}
			tmp[path] = to.StdDuration()
		}
	}
	this.handlerTimeout = tmp
}

func (this *CGrpcServer) getHandlerTimeout(path string) time.Duration {
	if t, ok := this.handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout.StdDuration()
}

// thread unsafe
func (s *CGrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *CGrpcServer) RegisterHandler(sname, mname string, handlers ...OutsideHandler) {
	service, ok := s.services[sname]
	if !ok {
		service = &grpc.ServiceDesc{
			ServiceName: sname,
			HandlerType: nil,
			Methods:     []grpc.MethodDesc{},
			Streams:     []grpc.StreamDesc{},
			Metadata:    nil,
		}
		s.services[sname] = service
	}
	service.Methods = append(service.Methods, grpc.MethodDesc{
		MethodName: mname,
		Handler:    s.insidehandler(sname, mname, handlers...),
	})
}
func (s *CGrpcServer) insidehandler(sname, mname string, handlers ...OutsideHandler) func(interface{}, context.Context, func(interface{}) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	path := "/" + sname + "/" + mname
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(_ interface{}, ctx context.Context, decode func(interface{}) error, _ grpc.UnaryServerInterceptor) (resp interface{}, e error) {
		gmd, ok := gmetadata.FromIncomingContext(ctx)
		if ok {
			if data := gmd.Get("Core-Target"); len(data) != 0 && data[0] != s.self {
				return nil, cerror.ErrTarget
			}
		}
		atomic.AddInt64(&s.totalreqnum, 1)
		defer atomic.AddInt64(&s.totalreqnum, -1)
		//trace
		conninfo := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
		remoteaddr := conninfo.RemoteAddr.String()
		localaddr := conninfo.LocalAddr.String()
		sourceip := ""
		if ok {
			if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
				sourceip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
			} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
				sourceip = strings.TrimSpace(realip[0])
			}
		}
		if sourceip == "" {
			sourceip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
		}
		sourceapp := "unknown"
		sourcemethod := "unknown"
		sourcepath := "unknown"
		if ok {
			if data := gmd.Get("Core-Tracedata"); len(data) == 0 || data[0] == "" {
				ctx = log.InitTrace(ctx, "", s.self, host.Hostip, "GRPC", path, 0)
			} else if len(data) != 5 {
				log.Error(nil, "[cgrpc.server] tracedata format wrong", log.String("cip", sourceip), log.String("path", path), log.Any("tracedata", data))
				return nil, cerror.ErrReq
			} else {
				sourceapp = data[1]
				sourcemethod = data[2]
				sourcepath = data[3]
				clientdeep, e := strconv.Atoi(data[4])
				if e != nil || sourceapp == "" || sourcemethod == "" || sourcepath == "" || clientdeep == 0 {
					log.Error(nil, "[cgrpc.server] tracedata format wrong", log.String("cip", sourceip), log.String("path", path), log.Any("tracedata", data))
					return nil, cerror.ErrReq
				}
				ctx = log.InitTrace(ctx, data[0], s.self, host.Hostip, "GRPC", path, clientdeep)
			}
		}
		traceid, _, _, _, _, selfdeep := log.GetTrace(ctx)
		//metadata
		var cmd map[string]string
		if ok {
			data := gmd.Get("Core-Metadata")
			if len(data) != 0 {
				cmd = make(map[string]string)
				if e := json.Unmarshal(common.Str2byte(data[0]), &cmd); e != nil {
					log.Error(ctx, "[cgrpc.server] metadata format wrong",
						log.String("cname", sourceapp),
						log.String("cip", sourceip),
						log.String("path", path),
						log.String("metadata", data[0]))
					return nil, cerror.ErrReq
				}
			}
		}
		if cmd == nil {
			cmd = map[string]string{"Client-IP": sourceip}
		} else if _, ok := cmd["Client-IP"]; !ok {
			cmd["Client-IP"] = sourceip
		}
		//timeout
		start := time.Now()
		servertimeout := s.getHandlerTimeout(path)
		if servertimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, start.Add(servertimeout))
			defer cancel()
		}
		workctx := s.getcontext(cmetadata.SetMetadata(ctx, cmd), path, sourceapp, remoteaddr, totalhandlers, decode)
		paniced := true
		defer func() {
			if paniced {
				e := recover()
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[cgrpc.server] panic",
					log.String("cname", sourceapp),
					log.String("cip", sourceip),
					log.String("path", path),
					log.Any("panic", e),
					log.String("stack", base64.StdEncoding.EncodeToString(stack[:n])))
				workctx.e = cerror.ErrPanic
				workctx.resp = nil
			}

			grpc.SetTrailer(ctx, gmetadata.New(map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}))
			end := time.Now()
			log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.self, host.Hostip+":"+localaddr[strings.LastIndex(localaddr, ":")+1:], "GRPC", path, &start, &end, workctx.e)
			monitor.GrpcServerMonitor(sourceapp, "GRPC", path, workctx.e, uint64(end.UnixNano()-start.UnixNano()))
			resp = workctx.resp
			if workctx.e != nil {
				e = workctx.e
			}
		}()
		workctx.run()
		paniced = false
		return
	}
}

type serverrpckey struct{}

func (s *CGrpcServer) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, serverrpckey{}, info)
}
func (s *CGrpcServer) HandleRPC(context.Context, stats.RPCStats) {
}

type serverconnkey struct{}

func (s *CGrpcServer) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, serverconnkey{}, info)
}
func (s *CGrpcServer) HandleConn(ctx context.Context, stat stats.ConnStats) {
	info, ok := ctx.Value(serverconnkey{}).(*stats.ConnTagInfo)
	if !ok {
		return
	}
	gmd, ok := gmetadata.FromIncomingContext(ctx)
	peerip := ""
	if ok {
		if forward := gmd.Get("X-Forwarded-For"); len(forward) > 0 && len(forward[0]) > 0 {
			peerip = strings.TrimSpace(strings.Split(forward[0], ",")[0])
		} else if realip := gmd.Get("X-Real-Ip"); len(realip) > 0 && len(realip[0]) > 0 {
			peerip = strings.TrimSpace(realip[0])
		}
	}
	if peerip == "" {
		remoteaddr := info.RemoteAddr.String()
		peerip = remoteaddr[:strings.LastIndex(remoteaddr, ":")]
	}
	switch stat.(type) {
	case *stats.ConnBegin:
		atomic.AddInt32(&s.clientnum, 1)
		log.Info(nil, "[cgrpc.server] online", log.String("cip", peerip))
	case *stats.ConnEnd:
		atomic.AddInt32(&s.clientnum, -1)
		log.Info(nil, "[cgrpc.server] offline", log.String("cip", peerip))
	}
}
