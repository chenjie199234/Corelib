package monitor

import (
	"encoding/json"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/host"
)

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
	initmem()
	initcpu()
	initclient()
	initserver()
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			routinenum, threadnum, gctime := golangCollect()
			lastcpu, maxcpu, averagecpu := cpuCollect()
			lastmem, maxmem := memCollect()
			webc, grpcc, crpcc := clientCollect()
			webs, grpcs, crpcs := serverCollect()
			buf := pool.GetBuffer()

			buf.AppendString("# HELP gc_time\n")
			buf.AppendString("# TYPE gc_time gauge\n")
			buf.AppendString("gc_time ")
			buf.AppendFloat64(float64(gctime) / 1000 / 1000) //transfer uint to ms
			buf.AppendByte('\n')

			buf.AppendString("# HELP routine_num\n")
			buf.AppendString("# TYPE routine_num gauge\n")
			buf.AppendString("routine_num ")
			buf.AppendUint64(routinenum)
			buf.AppendByte('\n')

			buf.AppendString("# HELP thread_num\n")
			buf.AppendString("# TYPE thread_num gauge\n")
			buf.AppendString("thread_num ")
			buf.AppendUint64(threadnum)
			buf.AppendByte('\n')

			buf.AppendString("# HELP cpu")
			buf.AppendString("# TYPE cpu gauge")
			//cur
			buf.AppendString("cpu{id=\"cur\"} ")
			buf.AppendFloat64(lastcpu)
			buf.AppendByte('\n')
			//max
			buf.AppendString("cpu{id=\"max\"} ")
			buf.AppendFloat64(maxcpu)
			buf.AppendByte('\n')
			//avg
			buf.AppendString("cpu{id=\"avg\"} ")
			buf.AppendFloat64(averagecpu)
			buf.AppendByte('\n')

			buf.AppendString("# HELP mem")
			buf.AppendString("# TYPE mem gauge")
			//cur
			buf.AppendString("mem{id=\"cur\"} ")
			buf.AppendFloat64(lastmem)
			buf.AppendByte('\n')
			//max
			buf.AppendString("mem{id=\"max\"} ")
			buf.AppendFloat64(maxmem)
			buf.AppendByte('\n')

			if len(webc) > 0 {
				buf.AppendString("# HELP web_client_call_time\n")
				buf.AppendString("# TYPE web_client_call_time summary\n")
				for peername, peer := range webc {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("web_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("web_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# TYPE web_client_call_err guage\n")
				for peername, peer := range webc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("web_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}

			if len(webs) > 0 {
				buf.AppendString("# HELP web_server_call_time\n")
				buf.AppendString("# HELP web_server_call_time summary\n")
				for peername, peer := range webs {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("web_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("web_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# HELP web_server_call_err guage\n")
				for peername, peer := range webs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("web_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}

			if len(grpcc) > 0 {
				buf.AppendString("# HELP grpc_client_call_time\n")
				buf.AppendString("# HELP grpc_client_call_time summary\n")
				for peername, peer := range grpcc {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("grpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("grpc_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# HELP grpc_client_call_err guage\n")
				for peername, peer := range grpcc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("grpc_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}

			if len(grpcs) > 0 {
				buf.AppendString("# HELP grpc_server_call_time\n")
				buf.AppendString("# HELP grpc_server_call_time summary\n")
				for peername, peer := range grpcs {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("grpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("grpc_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# HELP grpc_server_call_err guage\n")
				for peername, peer := range grpcs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("grpc_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}

			if len(crpcc) > 0 {
				buf.AppendString("# HELP crpc_client_time\n")
				buf.AppendString("# HELP crpc_client_time summary\n")
				for peername, peer := range crpcc {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("crpc_client_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("crpc_client_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# HELP crpc_client_err guage\n")
				for peername, peer := range crpcc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("crpc_client_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}

			if len(crpcs) > 0 {
				buf.AppendString("# HELP crpc_server_time\n")
				buf.AppendString("# HELP crpc_server_time summary\n")
				for peername, peer := range crpcs {
					for pathname, path := range peer {
						//50 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.50\"} ")
						buf.AppendFloat64(float64(path.T50) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//90 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.90\"} ")
						buf.AppendFloat64(float64(path.T90) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//99 percent
						buf.AppendString("crpc_server_call_time{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\",quantile=\"0.99\"} ")
						buf.AppendFloat64(float64(path.T99) / 1000 / 1000) //transfer uint to ms
						buf.AppendByte('\n')
						//sum
						buf.AppendString("crpc_server_call_time_sum{peer=\"")
						buf.AppendString(peername)
						buf.AppendString("\",path=\"")
						buf.AppendString(pathname)
						buf.AppendString("\"} ")
						buf.AppendFloat64(float64(path.TotaltimeWaste) / 1000 / 1000) //transfer uint to ms
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
				buf.AppendString("# HELP crpc_server_err guage\n")
				for peername, peer := range crpcs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf.AppendString("crpc_server_call_err{peer=\"")
							buf.AppendString(peername)
							buf.AppendString("\",path=\"")
							buf.AppendString(pathname)
							buf.AppendString("\",ecode=\"")
							buf.AppendInt32(errcode)
							buf.AppendString("\"} ")
							buf.AppendUint32(errcount)
							buf.AppendByte('\n')
						}
					}
				}
			}
			w.Write(buf.Bytes())
			pool.PutBuffer(buf)
		})
		http.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
			d, _ := json.Marshal(&struct {
				HostIP   string  `json:"host_ip"`
				HostName string  `json:"host_name"`
				CpuNum   float64 `json:"cpu_num"`
				CpuUsage float64 `json:"cur_usage"`
				MemTotal uint64  `json:"mem_total"`
				MemUsage float64 `json:"mem_usage"`
			}{
				HostIP:   host.Hostip,
				HostName: host.Hostname,
				CpuNum:   CPUNum,
				CpuUsage: LastUsageCPU,
				MemTotal: uint64(TotalMEM),
				MemUsage: LastUsageMEM,
			})
			w.Header().Set("Content-Type", "application/json")
			w.Write(d)
		})
		http.ListenAndServe(":6060", nil)
	}()
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

type pathinfo struct {
	TotalCount     uint32
	ErrCodeCount   map[int32]uint32 //key:error code,value:count
	TotaltimeWaste uint64           //nano second
	maxTimewaste   uint64           //nano second
	timewaste      [5001]uint32     //index:0-4999(1ms-5000ms) each 1ms,index:5000 more then 5s,value:count
	lker           *sync.Mutex
	T50            uint64 //nano second
	T90            uint64 //nano second
	T99            uint64 //nano second
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
