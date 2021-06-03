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
	if e := s.CreateGroup("testgroup", "testname", "xxx", buildcmds, runcmd); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if ginfo.Status == g_CLOSING {
				panic("init group failed")
			} else if ginfo.Status == g_BUILDFAILED {
				break
			}
		}
	}
	if e := s.BuildGroup("testgroup", "testname", "ajdklasdklbnakshd", ""); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if ginfo.Status == g_BUILDFAILED {
				t.Log("build failed")
				break
			} else if ginfo.Status == g_BUILDSUCCESS {
				panic("should build failed but build success")
			}
		}
	}
	if e := s.BuildGroup("testgroup", "testname", "master", ""); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if ginfo.Status == g_BUILDSUCCESS {
				t.Log("build success")
				d, _ := json.Marshal(ginfo)
				t.Log(string(d))
				break
			} else if ginfo.Status == g_BUILDFAILED {
				panic("should build success but build failed")
			}
		}
	}
	if e := s.UpdateGroup("testgroup", "testname"); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if ginfo.Status == g_UPDATESUCCESS || ginfo.Status == g_BUILDSUCCESS {
				t.Log("update success")
				d, _ := json.Marshal(ginfo)
				t.Log(string(d))
				break
			} else if ginfo.Status == g_UPDATEFAILED {
				panic("update failed")
			}
		}
	}
	ppid := 0
	if e := s.StartProcess("testgroup", "testname", true); e != nil {
		panic("start error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if len(ginfo.Pinfo) > 0 {
				if ginfo.Pinfo[0].Status == p_STARTING {
					t.Log("process is starting")
				}
				if ginfo.Pinfo[0].Status == p_CLOSING {
					panic("start process error")
				}
				if ginfo.Pinfo[0].Status == p_WORKING {
					ppid = int(ginfo.Pinfo[0].Ppid)
					t.Log("process start success")
					d, _ := json.Marshal(ginfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	_ = ppid
	if e := s.RestartProcess("testgroup", "testname", 1); e != nil {
		panic("restart error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if len(ginfo.Pinfo) > 0 {
				if ginfo.Pinfo[0].Status == p_WORKING && ppid != int(ginfo.Pinfo[0].Ppid) {
					ppid = int(ginfo.Pinfo[0].Ppid)
					t.Log("restart success")
					d, _ := json.Marshal(ginfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	if e := s.StopProcess("testgroup", "testname", 1); e != nil {
		panic("stop error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if len(ginfo.Pinfo) == 0 {
				t.Log("stop success")
				break
			}
		}
	}
	if e := s.StartProcess("testgroup", "testname", true); e != nil {
		panic("start error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("testgroup", "testname"); e == nil {
			if len(ginfo.Pinfo) > 0 {
				if ginfo.Pinfo[0].Status == p_STARTING {
					t.Log("process is starting")
				}
				if ginfo.Pinfo[0].Status == p_CLOSING {
					panic("start process error")
				}
				if ginfo.Pinfo[0].Status == p_WORKING {
					ppid = int(ginfo.Pinfo[0].Ppid)
					t.Log("process start success")
					d, _ := json.Marshal(ginfo)
					t.Log(string(d))
					break
				}
			}
		}
	}
	if e := s.DeleteGroup("testgroup", "testname"); e != nil {
		panic("del error:" + e.Error())
	}
	for {
		time.Sleep(time.Second)
		ginfo, e := s.GetGroupInfo("testgroup", "testname")
		if e != nil && e.Error() == "[groupinfo] group doesn't exist" {
			t.Log("deleted")
			break
		} else {
			t.Log("not deleted")
			d, _ := json.Marshal(ginfo)
			t.Log(string(d))
		}
	}
}
