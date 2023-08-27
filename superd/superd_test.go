package superd

import (
	"encoding/json"
	"testing"
	"time"
)

func Test_Super(t *testing.T) {
	s := NewSuper()
	buildcmds := make([]*Cmd, 0)
	buildcmds = append(buildcmds, &Cmd{
		Cmd:  "go",
		Args: []string{"mod", "tidy"},
	})
	buildcmds = append(buildcmds, &Cmd{
		Cmd:  "go",
		Args: []string{"build", "-o", "main"},
	})
	runcmd := &Cmd{
		Cmd:  "./main",
		Args: nil,
	}
	if e := s.CreateApp("project", "group", "app", "xxx", buildcmds, runcmd); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if ainfo.Status == a_CLOSING {
				panic("init failed")
			} else if ainfo.Status == a_BUILDFAILED {
				break
			}
		}
	}
	if e := s.BuildApp("project", "group", "name", "ajdklasdklbnakshd"); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if ainfo.Status == a_BUILDFAILED {
				t.Log("build failed")
				break
			} else if ainfo.Status == a_BUILDSUCCESS {
				panic("should build failed but build success")
			}
		}
	}
	if e := s.BuildApp("project", "group", "name", "master"); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if ainfo.Status == a_BUILDSUCCESS {
				t.Log("build success")
				d, _ := json.Marshal(ainfo)
				t.Log(string(d))
				break
			} else if ainfo.Status == a_BUILDFAILED {
				panic("should build success but build failed")
			}
		}
	}
	if e := s.UpdateApp("project", "group", "name"); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if ainfo.Status == a_UPDATESUCCESS || ainfo.Status == a_BUILDSUCCESS {
				t.Log("update success")
				d, _ := json.Marshal(ainfo)
				t.Log(string(d))
				break
			} else if ainfo.Status == a_UPDATEFAILED {
				panic("update failed")
			}
		}
	}
	ppid := 0
	if e := s.StartAppProcess("project", "group", "name", true); e != nil {
		panic("start error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if len(ainfo.ProcessInfo) > 0 {
				if ainfo.ProcessInfo[0].Status == p_STARTING {
					t.Log("process is starting")
				}
				if ainfo.ProcessInfo[0].Status == p_CLOSING {
					panic("start process error")
				}
				if ainfo.ProcessInfo[0].Status == p_WORKING {
					ppid = int(ainfo.ProcessInfo[0].Ppid)
					t.Log("process start success")
					d, _ := json.Marshal(ainfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	_ = ppid
	if e := s.RestartAppProcess("project", "group", "name", 1); e != nil {
		panic("restart error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if len(ainfo.ProcessInfo) > 0 {
				if ainfo.ProcessInfo[0].Status == p_WORKING && ppid != int(ainfo.ProcessInfo[0].Ppid) {
					ppid = int(ainfo.ProcessInfo[0].Ppid)
					t.Log("restart success")
					d, _ := json.Marshal(ainfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	if e := s.StopAppProcess("project", "group", "name", 1); e != nil {
		panic("stop error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if len(ainfo.ProcessInfo) == 0 {
				t.Log("stop success")
				break
			}
		}
	}
	if e := s.StartAppProcess("project", "group", "name", true); e != nil {
		panic("start error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ainfo, e := s.GetAppInfo("project", "group", "name"); e == nil {
			if len(ainfo.ProcessInfo) > 0 {
				if ainfo.ProcessInfo[0].Status == p_STARTING {
					t.Log("process is starting")
				}
				if ainfo.ProcessInfo[0].Status == p_CLOSING {
					panic("start process error")
				}
				if ainfo.ProcessInfo[0].Status == p_WORKING {
					ppid = int(ainfo.ProcessInfo[0].Ppid)
					t.Log("process start success")
					d, _ := json.Marshal(ainfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	if e := s.DeleteApp("project", "group", "name"); e != nil {
		panic("del error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		ainfo, e := s.GetAppInfo("project", "group", "name")
		if e != nil && e.Error() == "[GetAppInfo] app doesn't exist" {
			t.Log("deleted")
			break
		} else {
			t.Log("not deleted")
			d, _ := json.Marshal(ainfo)
			t.Log(string(d))
		}
	}
}
