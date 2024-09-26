package monitor

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

func init() {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		panic("[monitor] unsupported GOOS")
	}
	if str := os.Getenv("MONITOR"); str != "" && str != "<MONITOR>" && str != "0" && str != "1" {
		panic("[monitor] os env MONITOR error,must in [0,1]")
	} else if str != "1" {
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
			totalmem, lastmem, maxmem := memCollect()
			webc, grpcc, crpcc := clientCollect()
			webs, grpcs, crpcs := serverCollect()
			buf := make([]byte, 0, 65536)

			buf = append(buf, "# HELP gc_time\n"...)
			buf = append(buf, "# TYPE gc_time gauge\n"...)
			buf = append(buf, "gc_time "...)
			buf = strconv.AppendFloat(buf, float64(gctime)/1000/1000, 'f', -1, 64) //transfer uint to ms
			buf = append(buf, '\n')

			buf = append(buf, "# HELP routine_num\n"...)
			buf = append(buf, "# TYPE routine_num gauge\n"...)
			buf = append(buf, "routine_num "...)
			buf = strconv.AppendUint(buf, routinenum, 10)
			buf = append(buf, '\n')

			buf = append(buf, "# HELP thread_num\n"...)
			buf = append(buf, "# TYPE thread_num gauge\n"...)
			buf = append(buf, "thread_num "...)
			buf = strconv.AppendUint(buf, threadnum, 10)
			buf = append(buf, '\n')

			buf = append(buf, "# HELP cpu\n"...)
			buf = append(buf, "# TYPE cpu gauge\n"...)
			//cur
			buf = append(buf, "cpu{id=\"cur\"} "...)
			buf = strconv.AppendFloat(buf, lastcpu, 'f', -1, 64)
			buf = append(buf, '\n')
			//max
			buf = append(buf, "cpu{id=\"max\"} "...)
			buf = strconv.AppendFloat(buf, maxcpu, 'f', -1, 64)
			buf = append(buf, '\n')
			//avg
			buf = append(buf, "cpu{id=\"avg\"} "...)
			buf = strconv.AppendFloat(buf, averagecpu, 'f', -1, 64)
			buf = append(buf, '\n')

			buf = append(buf, "# HELP mem\n"...)
			buf = append(buf, "# TYPE mem gauge\n"...)
			//total
			buf = append(buf, "mem{id=\"total\"} "...)
			buf = strconv.AppendUint(buf, totalmem, 10)
			buf = append(buf, '\n')
			//cur
			buf = append(buf, "mem{id=\"cur\"} "...)
			buf = strconv.AppendUint(buf, lastmem, 10)
			buf = append(buf, '\n')
			//max
			buf = append(buf, "mem{id=\"max\"} "...)
			buf = strconv.AppendUint(buf, maxmem, 10)
			buf = append(buf, '\n')

			if len(webc) > 0 {
				buf = append(buf, "# HELP web_client_call_time\n"...)
				buf = append(buf, "# TYPE web_client_call_time summary\n"...)
				for peername, peer := range webc {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "web_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "web_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "web_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "web_client_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "web_client_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP web_client_call_err\n"...)
				buf = append(buf, "# TYPE web_client_call_err guage\n"...)
				for peername, peer := range webc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "web_client_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}

			if len(webs) > 0 {
				buf = append(buf, "# HELP web_server_call_time\n"...)
				buf = append(buf, "# HELP web_server_call_time summary\n"...)
				for peername, peer := range webs {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "web_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "web_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "web_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "web_server_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "web_server_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP web_server_call_err\n"...)
				buf = append(buf, "# HELP web_server_call_err guage\n"...)
				for peername, peer := range webs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "web_server_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}

			if len(grpcc) > 0 {
				buf = append(buf, "# HELP grpc_client_call_time\n"...)
				buf = append(buf, "# HELP grpc_client_call_time summary\n"...)
				for peername, peer := range grpcc {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "grpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "grpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "grpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "grpc_client_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "grpc_client_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP grpc_client_call_err\n"...)
				buf = append(buf, "# HELP grpc_client_call_err guage\n"...)
				for peername, peer := range grpcc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "grpc_client_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}

			if len(grpcs) > 0 {
				buf = append(buf, "# HELP grpc_server_call_time\n"...)
				buf = append(buf, "# HELP grpc_server_call_time summary\n"...)
				for peername, peer := range grpcs {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "grpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "grpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "grpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "grpc_server_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "grpc_server_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP grpc_server_call_err\n"...)
				buf = append(buf, "# HELP grpc_server_call_err guage\n"...)
				for peername, peer := range grpcs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "grpc_server_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}

			if len(crpcc) > 0 {
				buf = append(buf, "# HELP crpc_client_time\n"...)
				buf = append(buf, "# HELP crpc_client_time summary\n"...)
				for peername, peer := range crpcc {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "crpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "crpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "crpc_client_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "crpc_client_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "crpc_client_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP crpc_client_err\n"...)
				buf = append(buf, "# HELP crpc_client_err guage\n"...)
				for peername, peer := range crpcc {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "crpc_client_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}

			if len(crpcs) > 0 {
				buf = append(buf, "# HELP crpc_server_time\n"...)
				buf = append(buf, "# HELP crpc_server_time summary\n"...)
				for peername, peer := range crpcs {
					for pathname, path := range peer {
						t50, t90, t99 := path.getT()
						//50 percent
						buf = append(buf, "crpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.50\"} "...)
						buf = strconv.AppendFloat(buf, float64(t50)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//90 percent
						buf = append(buf, "crpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.90\"} "...)
						buf = strconv.AppendFloat(buf, float64(t90)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//99 percent
						buf = append(buf, "crpc_server_call_time{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\",quantile=\"0.99\"} "...)
						buf = strconv.AppendFloat(buf, float64(t99)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//sum
						buf = append(buf, "crpc_server_call_time_sum{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendFloat(buf, float64(path.TotaltimeWaste)/1000/1000, 'f', -1, 64) //transfer uint to ms
						buf = append(buf, '\n')
						//count
						buf = append(buf, "crpc_server_call_time_count{peer=\""...)
						buf = append(buf, peername...)
						buf = append(buf, "\",path=\""...)
						buf = append(buf, pathname...)
						buf = append(buf, "\"} "...)
						buf = strconv.AppendUint(buf, uint64(path.TotalCount), 10)
						buf = append(buf, '\n')
					}
				}
				buf = append(buf, "# HELP crpc_server_err\n"...)
				buf = append(buf, "# HELP crpc_server_err guage\n"...)
				for peername, peer := range crpcs {
					for pathname, path := range peer {
						for errcode, errcount := range path.ErrCodeCount {
							buf = append(buf, "crpc_server_call_err{peer=\""...)
							buf = append(buf, peername...)
							buf = append(buf, "\",path=\""...)
							buf = append(buf, pathname...)
							buf = append(buf, "\",ecode=\""...)
							buf = strconv.AppendInt(buf, int64(errcode), 10)
							buf = append(buf, "\"} "...)
							buf = strconv.AppendUint(buf, uint64(errcount), 10)
							buf = append(buf, '\n')
						}
					}
				}
			}
			w.Write(buf)
		})
		http.ListenAndServe(":6060", nil)
	}()
}

// timewaste nanosecond
func index(timewaste uint64) int64 {
	i := timewaste / 1000000
	if i >= 5000 {
		return 5000
	}
	return int64(i)
}

type pathinfo struct {
	TotalCount     uint32
	ErrCodeCount   map[int32]uint32 //key:error code,value:count
	TotaltimeWaste uint64           //nano second
	maxTimewaste   uint64           //nano second
	timewaste      [5001]uint32     //index:0-4999(1ms-5000ms) each 1ms,index:5000 more then 5s,value:count
	lker           *sync.Mutex
}

func (p *pathinfo) getT() (uint64, uint64, uint64) {
	if p.TotalCount == 0 {
		return 0, 0, 0
	}
	var T50, T90, T99 uint64
	var T50Done, T90Done, T99Done bool
	T50Count := uint32(float64(p.TotalCount) * 0.5)
	T90Count := uint32(float64(p.TotalCount) * 0.9)
	T99Count := uint32(float64(p.TotalCount) * 0.99)
	var sum uint32
	var prefixtime uint64
	var timepiece uint64
	for index, count := range p.timewaste {
		if count == 0 {
			continue
		}
		if index <= 4999 {
			prefixtime = uint64(time.Millisecond) * uint64(index)
		} else {
			prefixtime = uint64(time.Second * 5)
		}
		if index <= 4999 {
			if p.maxTimewaste-prefixtime >= uint64(time.Millisecond) {
				timepiece = uint64(time.Millisecond)
			} else {
				timepiece = p.maxTimewaste - prefixtime
			}
		} else {
			timepiece = p.maxTimewaste - prefixtime
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
