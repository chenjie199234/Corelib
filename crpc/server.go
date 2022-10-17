package crpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"math"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	cerror "github.com/chenjie199234/Corelib/error"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/monitor"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/chenjie199234/Corelib/util/host"
	"github.com/chenjie199234/Corelib/util/name"

	"google.golang.org/protobuf/proto"
)

type OutsideHandler func(*Context)

type ServerConfig struct {
	GlobalTimeout  time.Duration     //global timeout for every rpc call
	ConnectTimeout time.Duration     //default 500ms
	HeartPorbe     time.Duration     //default 1s,3 probe missing means disconnect
	MaxMsgLen      uint32            //default 64M,min 64k
	Certs          map[string]string //mapkey: cert path,mapvalue: key path
}

func (c *ServerConfig) validate() {
	if c.GlobalTimeout < 0 {
		c.GlobalTimeout = 0
	}
}

type CrpcServer struct {
	c              *ServerConfig
	selfappname    string
	tlsc           *tls.Config
	global         []OutsideHandler
	ctxpool        *sync.Pool
	handler        map[string]func(context.Context, *stream.Peer, *Msg)
	handlerTimeout map[string]time.Duration
	instance       *stream.Instance
	closewait      *sync.WaitGroup
	totalreqnum    int32
}
type client struct {
	sync.RWMutex
	maxcallid uint64 //0-server is closing
	calls     map[uint64]context.CancelFunc
}

func NewCrpcServer(c *ServerConfig, selfgroup, selfname string) (*CrpcServer, error) {
	selfappname := selfgroup + "." + selfname
	if e := name.FullCheck(selfappname); e != nil {
		return nil, e
	}
	if c == nil {
		c = &ServerConfig{}
	}
	c.validate()
	serverinstance := &CrpcServer{
		c:              c,
		selfappname:    selfappname,
		global:         make([]OutsideHandler, 0, 10),
		ctxpool:        &sync.Pool{},
		handler:        make(map[string]func(context.Context, *stream.Peer, *Msg), 10),
		handlerTimeout: make(map[string]time.Duration),
		closewait:      &sync.WaitGroup{},
	}
	if len(c.Certs) != 0 {
		certificates := make([]tls.Certificate, 0, len(c.Certs))
		for cert, key := range c.Certs {
			temp, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, errors.New("[crpc.server] load cert:" + cert + " key:" + key + " error:" + e.Error())
			}
			certificates = append(certificates, temp)
		}
		serverinstance.tlsc = &tls.Config{Certificates: certificates}
	}
	serverinstance.closewait.Add(1)
	instancec := &stream.InstanceConfig{
		HeartprobeInterval: c.HeartPorbe,
		TcpC: &stream.TcpConfig{
			ConnectTimeout: c.ConnectTimeout,
			MaxMsgLen:      c.MaxMsgLen,
		},
	}
	instancec.VerifyFunc = serverinstance.verifyfunc
	instancec.OnlineFunc = serverinstance.onlinefunc
	instancec.UserdataFunc = serverinstance.userfunc
	instancec.OfflineFunc = serverinstance.offlinefunc
	serverinstance.instance, _ = stream.NewInstance(instancec)
	return serverinstance, nil
}

var ErrServerClosed = errors.New("[crpc.server] closed")

func (s *CrpcServer) StartCrpcServer(listenaddr string) error {
	e := s.instance.StartServer(listenaddr, s.tlsc)
	if e == stream.ErrServerClosed {
		return ErrServerClosed
	}
	return e
}
func (s *CrpcServer) StopCrpcServer() {
	defer func() {
		s.closewait.Wait()
		s.instance.Stop()
	}()
	s.instance.PreStop()
	var old int32
	for {
		old = atomic.LoadInt32(&s.totalreqnum)
		if old >= 0 {
			if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old-math.MaxInt32) {
				break
			}
		} else {
			return
		}
	}
	//tel all peers self closed
	s.instance.RangePeers(func(p *stream.Peer) {
		if tmpdata := p.GetData(); tmpdata != nil {
			c := (*client)(tmpdata)
			c.RLock()
			d, _ := proto.Marshal(&Msg{
				Callid: c.maxcallid + 1,
				Error:  cerror.ErrClosing,
			})
			c.maxcallid = 0
			p.SendMessage(nil, d, nil, nil)
			c.RUnlock()
		}
	})
	//if there are calls,done status will be checked when each call finishes
	//this is used to prevent there are no calls
	if old == 0 {
		s.closewait.Done()
	}
}
func (s *CrpcServer) GetClientNum() int32 {
	return s.instance.GetPeerNum()
}
func (s *CrpcServer) GetReqNum() int32 {
	totalreqnum := atomic.LoadInt32(&s.totalreqnum)
	if totalreqnum < 0 {
		return totalreqnum + math.MaxInt32
	} else {
		return totalreqnum
	}
}

// key path,value timeout(if timeout <= 0 means no timeout)
func (this *CrpcServer) UpdateHandlerTimeout(htcs map[string]time.Duration) {
	tmp := make(map[string]time.Duration)
	for path, timeout := range htcs {
		if len(path) == 0 || path[0] != '/' {
			path = "/" + path
		}
		tmp[path] = timeout
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout)), unsafe.Pointer(&tmp))
}

func (this *CrpcServer) getHandlerTimeout(path string) time.Duration {
	handlerTimeout := *(*map[string]time.Duration)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&this.handlerTimeout))))
	if t, ok := handlerTimeout[path]; ok {
		return t
	}
	return this.c.GlobalTimeout
}

// thread unsafe
func (s *CrpcServer) Use(globalMids ...OutsideHandler) {
	s.global = append(s.global, globalMids...)
}

// thread unsafe
func (s *CrpcServer) RegisterHandler(path string, handlers ...OutsideHandler) {
	s.handler[path] = s.insidehandler(path, handlers...)
}

func (s *CrpcServer) insidehandler(path string, handlers ...OutsideHandler) func(context.Context, *stream.Peer, *Msg) {
	totalhandlers := make([]OutsideHandler, len(s.global)+len(handlers))
	copy(totalhandlers, s.global)
	copy(totalhandlers[len(s.global):], handlers)
	return func(ctx context.Context, p *stream.Peer, msg *Msg) {
		traceid, _, _, _, _, selfdeep := log.GetTrace(ctx)
		var sourceapp, sourceip, sourcemethod, sourcepath string
		if msg.Tracedata != nil {
			sourceapp = msg.Tracedata["SourceApp"]
			sourcemethod = msg.Tracedata["SourceMethod"]
			sourcepath = msg.Tracedata["SourcePath"]
		}
		if sourceapp == "" {
			sourceapp = "unkown"
		}
		sourceip = p.GetRealPeerIp()
		if sourcemethod == "" {
			sourcemethod = "unknown"
		}
		if sourcepath == "" {
			sourcepath = "unknown"
		}
		//var globaldl int64
		start := time.Now()
		var min int64
		servertimeout := int64(s.getHandlerTimeout(path))
		if servertimeout > 0 {
			serverdl := time.Now().UnixNano() + servertimeout
			if msg.Deadline != 0 {
				//compare use the small one
				if msg.Deadline < serverdl {
					min = msg.Deadline
				} else {
					min = serverdl
				}
			} else {
				//use server timeout
				min = serverdl
			}
		} else if msg.Deadline != 0 {
			//use client timeout
			min = msg.Deadline
		} else {
			//no timeout
			min = 0
		}
		if min != 0 {
			if min < start.UnixNano()+int64(time.Millisecond) {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrDeadlineExceeded
				msg.Metadata = nil
				msg.Tracedata = nil
				end := time.Now()
				log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfappname, host.Hostip+":"+p.GetLocalPort(), "CRPC", path, &start, &end, msg.Error)
				monitor.CrpcServerMonitor(sourceapp, "CRPC", path, msg.Error, uint64(end.UnixNano()-start.UnixNano()))
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, time.Unix(0, min))
			defer cancel()
		}
		//logic
		workctx := s.getContext(ctx, p, msg, totalhandlers)
		if _, ok := workctx.metadata["Client-IP"]; !ok {
			workctx.metadata["Client-IP"] = sourceip
		}
		defer func() {
			if e := recover(); e != nil {
				stack := make([]byte, 1024)
				n := runtime.Stack(stack, false)
				log.Error(workctx, "[crpc.server] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "path:", path, "method: CRPC panic:", e, "stack:", base64.StdEncoding.EncodeToString(stack[:n]))
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrPanic
				msg.Metadata = nil
				msg.Tracedata = nil
			}
			end := time.Now()
			log.Trace(log.InitTrace(nil, traceid, sourceapp, sourceip, sourcemethod, sourcepath, selfdeep-1), log.SERVER, s.selfappname, host.Hostip+":"+p.GetLocalPort(), "CRPC", path, &start, &end, msg.Error)
			monitor.CrpcServerMonitor(sourceapp, "CRPC", path, msg.Error, uint64(end.UnixNano()-start.UnixNano()))
			s.putContext(workctx)
		}()
		workctx.run()
	}
}

// return false will close the connection
func (s *CrpcServer) verifyfunc(ctx context.Context, peerVerifyData []byte) ([]byte, bool) {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//self closed
		return nil, false
	}
	if common.Byte2str(peerVerifyData) != s.selfappname {
		return nil, false
	}
	return nil, true
}

// return false will close the connection
func (s *CrpcServer) onlinefunc(p *stream.Peer) bool {
	if atomic.LoadInt32(&s.totalreqnum) < 0 {
		//tel all peers self closed
		d, _ := proto.Marshal(&Msg{
			Callid: 0,
			Error:  cerror.ErrClosing,
		})
		p.SendMessage(nil, d, nil, nil)
	}
	c := &client{
		maxcallid: 1,
		calls:     make(map[uint64]context.CancelFunc),
	}
	p.SetData(unsafe.Pointer(c))
	log.Info(nil, "[crpc.server.onlinefunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "online")
	return true
}
func (s *CrpcServer) userfunc(p *stream.Peer, data []byte) {
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error(nil, "[crpc.server.userfunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "data format error:", e)
		p.Close()
		return
	}
	c := (*client)(p.GetData())
	if msg.Type == MsgType_CANCEL {
		c.RLock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
		}
		c.RUnlock()
		return
	}
	handler, ok := s.handler[msg.Path]
	if !ok {
		log.Error(nil, "[crpc.server.userfunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "call path:", msg.Path, "error: unknown path")
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrNoapi
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(nil, d, nil, nil); e != nil {
			log.Error(nil, "[crpc.server.userfunc] send message to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", e)
		}
		return
	}
	var tracectx context.Context
	if len(msg.Tracedata) == 0 || msg.Tracedata["Traceid"] == "" {
		tracectx = log.InitTrace(p, "", s.selfappname, host.Hostip, "CRPC", msg.Path, 0)
	} else if len(msg.Tracedata) != 4 || msg.Tracedata["Deep"] == "" {
		log.Error(nil, "[crpc.server.userfunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "path:", msg.Path, "method: CRPC error: tracedata:", msg.Tracedata, "format error")
		p.Close()
		return
	} else if clientdeep, e := strconv.Atoi(msg.Tracedata["Deep"]); e != nil {
		log.Error(nil, "[crpc.server.userfunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "path:", msg.Path, "method: CRPC error: tracedata:", msg.Tracedata, "format error")
		p.Close()
		return
	} else {
		tracectx = log.InitTrace(p, msg.Tracedata["Traceid"], s.selfappname, host.Hostip, "CRPC", msg.Path, clientdeep)
	}
	for {
		old := atomic.LoadInt32(&s.totalreqnum)
		if old < 0 {
			//tell peer self closed
			msg.Path = ""
			msg.Deadline = 0
			msg.Body = nil
			msg.Error = cerror.ErrClosing
			msg.Metadata = nil
			msg.Tracedata = nil
			d, _ := proto.Marshal(msg)
			if e := p.SendMessage(tracectx, d, nil, nil); e != nil {
				log.Error(tracectx, "[crpc.server.userfunc] send message to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", e)
			}
			return
		}
		//add req num
		if atomic.CompareAndSwapInt32(&s.totalreqnum, old, old+1) {
			break
		}
	}
	c.Lock()
	if c.maxcallid == 0 {
		c.Unlock()
		//tell peer self closed
		msg.Path = ""
		msg.Deadline = 0
		msg.Body = nil
		msg.Error = cerror.ErrClosing
		msg.Metadata = nil
		msg.Tracedata = nil
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(tracectx, d, nil, nil); e != nil {
			log.Error(tracectx, "[crpc.server.userfunc] send messge to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", e)
		}
		if atomic.AddInt32(&s.totalreqnum, -1) == -math.MaxInt32 {
			s.closewait.Done()
		}
		return
	}
	c.maxcallid = msg.Callid
	ctx, cancel := context.WithCancel(tracectx)
	c.calls[msg.Callid] = cancel
	c.Unlock()
	go func() {
		handler(ctx, p, msg)
		d, _ := proto.Marshal(msg)
		if e := p.SendMessage(ctx, d, nil, nil); e != nil {
			if e == stream.ErrMsgLarge {
				msg.Path = ""
				msg.Deadline = 0
				msg.Body = nil
				msg.Error = cerror.ErrRespmsgLen
				msg.Metadata = nil
				msg.Tracedata = nil
				d, _ = proto.Marshal(msg)
				if e = p.SendMessage(ctx, d, nil, nil); e != nil {
					log.Error(ctx, "[crpc.server.userfunc] send message to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", e)
				} else {
					log.Error(ctx, "[crpc.server.userfunc] send message to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", cerror.ErrRespmsgLen)
				}
			} else {
				log.Error(ctx, "[crpc.server.userfunc] send message to client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "error:", e)
			}
		}
		c.Lock()
		if cancel, ok := c.calls[msg.Callid]; ok {
			cancel()
			delete(c.calls, msg.Callid)
		}
		c.Unlock()
		if atomic.AddInt32(&s.totalreqnum, -1) == -math.MaxInt32 {
			s.closewait.Done()
		}
	}()
}
func (s *CrpcServer) offlinefunc(p *stream.Peer) {
	log.Info(nil, "[crpc.server.offlinefunc] client RemoteAddr:", p.GetRemoteAddr(), "RealIP:", p.GetRealPeerIp(), "offline")
}
