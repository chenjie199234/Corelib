package superd

import (
	"testing"
	"time"
)

func Test_Super(t *testing.T) {
	s, _ := NewSuper(&GroupConfig{SuperName: "super", LogRotateCap: 1, LogRotateCycle: 1, LogKeepDays: 1})
	buildcmds := make([]*Cmd, 0)
	buildcmds = append(buildcmds, &Cmd{
		Cmd:  "go",
		Args: []string{"build", "-o", "main"},
	})
	runcmd := &Cmd{
		Cmd:  "./main",
		Args: nil,
	}
	if e := s.CreateGroup("testgroup", "testname", "xxx", buildcmds, runcmd, 1); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("test"); e == nil {
			if ginfo.Status == g_CLOSING {
				panic("init group failed")
			} else if ginfo.Status == g_BUILDFAILED {
				break
			}
		}
	}
	if e := s.BuildGroup("test", "asdouaosjdl", ""); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("test"); e == nil {
			if ginfo.Status == g_CLOSING {
				panic("build group failed")
			}
			if ginfo.Status == g_BUILDFAILED {
				break
			}
			if ginfo.Status == g_BUILDSUCCESS {
				panic("build group failed")
			}
		}
	}
	if e := s.BuildGroup("test", "master", ""); e != nil {
		panic(e)
	}
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("test"); e == nil {
			if ginfo.Status == g_CLOSING {
				panic("build group failed")
			}
			if ginfo.Status == g_BUILDFAILED {
				panic("build group failed")
			}
			if ginfo.Status == g_BUILDSUCCESS {
				break
			}
		}
	}
	s.StartProcess("test", false)
	for {
		time.Sleep(time.Second)
		if ginfo, e := s.GetGroupInfo("test"); e == nil {
			if len(ginfo.Pinfo) > 0 {
				if ginfo.Pinfo[0].Status == p_STARTING {
					t.Log("process is starting")
				}
				if ginfo.Pinfo[0].Status == p_CLOSING {
					panic("start process error")
				}
				if ginfo.Pinfo[0].Status == p_WORKING {
					t.Log("process start success")
					break
				}
			}
		}
	}
	time.Sleep(time.Second * 5)
	//s.StopProcess("test", 1)
	//s.StopGroup("test")
	s.DeleteGroup("test")
	time.Sleep(time.Second)
}
