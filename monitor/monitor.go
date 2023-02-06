package monitor

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
)

var m monitor

var wclker sync.Mutex
var wslker sync.Mutex
var gclker sync.Mutex
var gslker sync.Mutex
var cclker sync.Mutex
var cslker sync.Mutex

type monitor struct {
	inited          bool
	Sysinfos        *sysinfo
	WebClientinfos  map[string]map[string]*pathinfo //first key peername,second key path
	WebServerinfos  map[string]map[string]*pathinfo //first key peername,second key path
	GrpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path
	GrpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path
	CrpcClientinfos map[string]map[string]*pathinfo //first key peername,second key path
	CrpcServerinfos map[string]map[string]*pathinfo //first key peername,second key path
}
type sysinfo struct {
	RoutineNum int
	ThreadNum  int
	HeapobjNum int
	GCTime     uint64
}
type pathinfo struct {
	TotalCount     uint32
	ErrCodeCount   map[int32]uint32
	TotaltimeWaste uint64       //nano second
	maxTimewaste   uint64       //nano second
	timewaste      [5001]uint32 //value:count,index:0-4999(1ms-5000ms) each 1ms,index:5000 more then 5s
	lker           *sync.Mutex
	T50            uint64 //nano second
	T90            uint64 //nano second
	T99            uint64 //nano second
}

func init() {
	if str := os.Getenv("MONITOR"); str == "" || str == "<MONITOR>" {
		log.Warning(nil, "[monitor] env MONITOR missing,monitor closed")
		return
	} else if n, e := strconv.Atoi(str); e != nil || n != 0 && n != 1 {
		log.Warning(nil, "[monitor] env MONITOR format error,must in [0,1],monitor closed")
		return
	} else if n == 0 {
		log.Warning(nil, "[monitor] env MONITOR is 0,monitor closed")
		return
	}
	refresh()
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			tmpm := getMonitorInfo()
			buf := pool.GetBuffer()
			buf.AppendString("# HELP heap_object_num\n")
			buf.AppendString("# TYPE heap_object_num gauge\n")
			buf.AppendString("heap_object_num ")
			buf.AppendInt(tmpm.Sysinfos.HeapobjNum)
			buf.AppendByte('\n')

			buf.AppendString("# HELP gc_time\n")
			buf.AppendString("# TYPE gc_time gauge\n")
			buf.AppendString("gc_time ")
			buf.AppendFloat64(float64(tmpm.Sysinfos.GCTime) / 1000 / 1000)
			buf.AppendByte('\n')

			buf.AppendString("# HELP routine_num\n")
			buf.AppendString("# TYPE routine_num gauge\n")
			buf.AppendString("routine_num ")
			buf.AppendInt(tmpm.Sysinfos.RoutineNum)
			buf.AppendByte('\n')

			buf.AppendString("# HELP thread_num\n")
			buf.AppendString("# TYPE thread_num gauge\n")
			buf.AppendString("thread_num ")
			buf.AppendInt(tmpm.Sysinfos.ThreadNum)
			buf.AppendByte('\n')

			if len(tmpm.WebClientinfos) > 0 {
				buf.AppendString("# HELP web_client_call_time\n")
				buf.AppendString("# TYPE web_client_call_time summary\n")
				for peername, peer := range tmpm.WebClientinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("web_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("web_client_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP web_client_call_err\n")
				buf.AppendString("# TYPE web_client_call_err summary\n")
				for peername, peer := range tmpm.WebClientinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("web_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("web_client_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("web_client_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}

			if len(tmpm.WebServerinfos) > 0 {
				buf.AppendString("# HELP web_server_call_time\n")
				buf.AppendString("# HELP web_server_call_time summary\n")
				for peername, peer := range tmpm.WebServerinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("web_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("web_server_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP web_server_call_err\n")
				buf.AppendString("# HELP web_server_call_err summary\n")
				for peername, peer := range tmpm.WebServerinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("web_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("web_server_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("web_server_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}

			if len(tmpm.GrpcClientinfos) > 0 {
				buf.AppendString("# HELP grpc_client_call_time\n")
				buf.AppendString("# HELP grpc_client_call_time summary\n")
				for peername, peer := range tmpm.GrpcClientinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("grpc_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("grpc_client_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP grpc_client_call_err\n")
				buf.AppendString("# HELP grpc_client_call_err summary\n")
				for peername, peer := range tmpm.GrpcClientinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("grpc_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("grpc_client_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("grpc_client_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}

			if len(tmpm.GrpcServerinfos) > 0 {
				buf.AppendString("# HELP grpc_server_call_time\n")
				buf.AppendString("# HELP grpc_server_call_time summary\n")
				for peername, peer := range tmpm.GrpcServerinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("grpc_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("grpc_server_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP grpc_server_call_err\n")
				buf.AppendString("# HELP grpc_server_call_err summary\n")
				for peername, peer := range tmpm.GrpcServerinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("grpc_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("grpc_server_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("grpc_server_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}

			if len(tmpm.CrpcClientinfos) > 0 {
				buf.AppendString("# HELP crpc_client_time\n")
				buf.AppendString("# HELP crpc_client_time summary\n")
				for peername, peer := range tmpm.CrpcClientinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("crpc_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("crpc_client_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP crpc_client_err\n")
				buf.AppendString("# HELP crpc_client_err summary\n")
				for peername, peer := range tmpm.CrpcClientinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("crpc_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("crpc_client_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("crpc_client_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}

			if len(tmpm.CrpcServerinfos) > 0 {
				buf.AppendString("# HELP crpc_server_time\n")
				buf.AppendString("# HELP crpc_server_time summary\n")
				for peername, peer := range tmpm.CrpcServerinfos {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000)
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000)
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000)
						buf.AppendByte('\n')
						//sum
						buf.AppendString("crpc_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000)
						buf.AppendByte('\n')
						//count
						buf.AppendString("crpc_server_call_time_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
				buf.AppendString("# HELP crpc_server_err\n")
				buf.AppendString("# HELP crpc_server_err summary\n")
				for peername, peer := range tmpm.CrpcServerinfos {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("crpc_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",quantile=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
						//sum
						buf.AppendString("crpc_server_call_err_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount - path.ErrCodeCount[0])
						buf.AppendByte('\n')
						//count
						buf.AppendString("crpc_server_call_err_count{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendUint32(path.TotalCount)
						buf.AppendByte('\n')
					}
				}
			}
			w.Write(buf.Bytes())
			pool.PutBuffer(buf)
		})
		http.ListenAndServe(":6060", nil)
	}()
}
func refresh() {
	m.inited = true
	m.WebClientinfos = make(map[string]map[string]*pathinfo)
	m.WebServerinfos = make(map[string]map[string]*pathinfo)
	m.GrpcClientinfos = make(map[string]map[string]*pathinfo)
	m.GrpcServerinfos = make(map[string]map[string]*pathinfo)
	m.CrpcClientinfos = make(map[string]map[string]*pathinfo)
	m.CrpcServerinfos = make(map[string]map[string]*pathinfo)
}

// timewaste nanosecond
func index(timewaste uint64) int64 {
	var ms int64
	if timewaste%1000000 > 0 {
		ms = int64(timewaste/1000000 + 1)
	} else {
		ms = int64(timewaste / 1000000)
	}
	if ms == 0 {
		ms = 1
	}
	if ms <= (time.Second * 5).Milliseconds() {
		return ms - 1
	}
	return 5000
}
func WebClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	wclker.Lock()
	peer, ok := m.WebClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.WebClientinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	wclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func WebServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	wslker.Lock()
	peer, ok := m.WebServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.WebServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	wslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func GrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	gclker.Lock()
	peer, ok := m.GrpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.GrpcClientinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	gclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func GrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	gslker.Lock()
	peer, ok := m.GrpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.GrpcServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	gslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcClientMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	cclker.Lock()
	peer, ok := m.CrpcClientinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.CrpcClientinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	cclker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}
func CrpcServerMonitor(peername, method, path string, e error, timewaste uint64) {
	if !m.inited {
		return
	}
	recordpath := method + ":" + path
	cslker.Lock()
	peer, ok := m.CrpcServerinfos[peername]
	if !ok {
		peer = make(map[string]*pathinfo)
		m.CrpcServerinfos[peername] = peer
	}
	pinfo, ok := peer[recordpath]
	if !ok {
		pinfo = &pathinfo{
			ErrCodeCount: make(map[int32]uint32),
			lker:         &sync.Mutex{},
		}
		peer[recordpath] = pinfo
	}
	pinfo.lker.Lock()
	cslker.Unlock()
	//timewaste
	if timewaste > pinfo.maxTimewaste {
		pinfo.maxTimewaste = timewaste
	}
	pinfo.TotaltimeWaste += timewaste
	pinfo.timewaste[index(timewaste)]++
	pinfo.TotalCount++
	//error
	ee := cerror.ConvertStdError(e)
	if ee == nil {
		pinfo.ErrCodeCount[0]++
	} else {
		pinfo.ErrCodeCount[ee.Code]++
	}
	pinfo.lker.Unlock()
}

var prefixGCwastetime uint64

func getMonitorInfo() *monitor {
	wclker.Lock()
	wslker.Lock()
	gclker.Lock()
	gslker.Lock()
	cclker.Lock()
	cslker.Lock()
	r := &monitor{
		Sysinfos:        &sysinfo{},
		WebClientinfos:  m.WebClientinfos,
		WebServerinfos:  m.WebServerinfos,
		GrpcClientinfos: m.GrpcClientinfos,
		GrpcServerinfos: m.GrpcServerinfos,
		CrpcClientinfos: m.CrpcClientinfos,
		CrpcServerinfos: m.CrpcServerinfos,
	}
	refresh()
	wclker.Unlock()
	wslker.Unlock()
	gclker.Unlock()
	gslker.Unlock()
	cclker.Unlock()
	cslker.Unlock()
	for _, peer := range r.WebClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.WebServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.GrpcClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.GrpcServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.CrpcClientinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	for _, peer := range r.CrpcServerinfos {
		for _, path := range peer {
			path.T50, path.T90, path.T99 = getT(&path.timewaste, path.maxTimewaste, path.TotalCount)
		}
	}
	r.Sysinfos.RoutineNum = runtime.NumGoroutine()
	r.Sysinfos.ThreadNum, _ = runtime.ThreadCreateProfile(nil)
	meminfo := &runtime.MemStats{}
	runtime.ReadMemStats(meminfo)
	r.Sysinfos.HeapobjNum = int(meminfo.HeapObjects)
	r.Sysinfos.GCTime = meminfo.PauseTotalNs - prefixGCwastetime
	prefixGCwastetime = meminfo.PauseTotalNs
	return r
}
func getT(data *[5001]uint32, maxtimewaste uint64, totalcount uint32) (uint64, uint64, uint64) {
	if totalcount == 0 {
		return 0, 0, 0
	}
	var T50, T90, T99 uint64
	var T50Done, T90Done, T99Done bool
	T50Count := uint32(float64(totalcount) * 0.5)
	T90Count := uint32(float64(totalcount) * 0.9)
	T99Count := uint32(float64(totalcount) * 0.99)
	var sum uint32
	var prefixtime uint64
	var timepiece uint64
	for index, count := range *data {
		if count == 0 {
			continue
		}
		if index <= 4999 {
			prefixtime = uint64(time.Millisecond) * uint64(index)
		} else {
			prefixtime = uint64(time.Second * 5)
		}
		if index <= 4999 {
			if maxtimewaste-prefixtime >= uint64(time.Millisecond) {
				timepiece = uint64(time.Millisecond)
			} else {
				timepiece = maxtimewaste - prefixtime
			}
		} else {
			timepiece = maxtimewaste - prefixtime
		}
		if sum+count >= T99Count && !T99Done {
			T99 = prefixtime + uint64(float64(timepiece)*(float64(T99Count-sum)/float64(count)))
			T99Done = true
		}
		if sum+count >= T90Count && !T90Done {
			T90 = prefixtime + uint64(float64(timepiece)*(float64(T90Count-sum)/float64(count)))
			T90Done = true
		}
		if sum+count >= T50Count && !T50Done {
			T50 = prefixtime + uint64(float64(timepiece)*(float64(T50Count-sum)/float64(count)))
			T50Done = true
		}
		if T99Done && T90Done && T50Done {
			break
		}
		sum += count
	}
	return T50, T90, T99
}
