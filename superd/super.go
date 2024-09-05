package superd

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/name"
)

const (
	s_CLOSING = iota
	s_WORKING
)

// struct
type Super struct {
	processid uint64
	lker      *sync.RWMutex
	apps      map[string]*app
	status    int
	notice    chan string
	closech   chan struct{}
}

// maxlogsize unit M
func NewSuper() *Super {
	instance := &Super{
		lker:    new(sync.RWMutex),
		apps:    make(map[string]*app, 5),
		status:  s_WORKING,
		notice:  make(chan string, 100),
		closech: make(chan struct{}, 1),
	}
	go func() {
		defer func() {
			instance.closech <- struct{}{}
		}()
		for {
			select {
			case fullappname, ok := <-instance.notice:
				if !ok {
					return
				}
				instance.lker.Lock()
				delete(instance.apps, fullappname)
				if instance.status == s_CLOSING && len(instance.apps) == 0 {
					instance.lker.Unlock()
					return
				}
				instance.lker.Unlock()
			}
		}
	}()
	return instance
}
func (s *Super) CloseSuper() {
	s.lker.Lock()
	if s.status == s_CLOSING {
		s.lker.Unlock()
		return
	}
	s.status = s_CLOSING
	if len(s.apps) == 0 {
		close(s.notice)
	} else {
		for _, a := range s.apps {
			go a.stopApp(true)
		}
	}
	s.lker.Unlock()
	<-s.closech
	return
}
func dirop(path string) error {
	finfo, e := os.Lstat(path)
	if e != nil {
		if !os.IsNotExist(e) {
			return errors.New("get " + path + " dir info error:" + e.Error())
		}
		if e = os.MkdirAll(path, 0755); e != nil {
			return errors.New(path + " dir not exist,and create error:" + e.Error())
		}
	} else if !finfo.IsDir() {
		return errors.New(path + " already exist and it's not a dir")
	}
	return nil
}

type Cmd struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
	Env  []string `json:"env"`
}

func (s *Super) CreateApp(project, groupname, appname, url string, buildcmds []*Cmd, runcmd *Cmd) error {
	fullappname, e := name.MakeFullName(project, groupname, appname)
	if e != nil {
		return e
	}
	s.lker.Lock()
	defer s.lker.Unlock()
	if s.status == s_CLOSING {
		return errors.New("[CreateApp] Superd is closing")
	}
	a, ok := s.apps[fullappname]
	if !ok {
		var e error
		if e = os.RemoveAll("./app_log/" + fullappname); e != nil {
			return errors.New("[CreateApp] remove old dir error:" + e.Error())
		}
		if e = os.RemoveAll("./app/" + fullappname); e != nil {
			return errors.New("[CreateApp] remote old dir error:" + e.Error())
		}
		if e = dirop("./app_log/" + fullappname); e != nil {
			return errors.New("[CreateApp] " + e.Error())
		}
		if e = dirop("./app/" + fullappname); e != nil {
			return errors.New("[CreateApp] " + e.Error())
		}
		a = &app{
			s:         s,
			project:   project,
			group:     groupname,
			app:       appname,
			url:       url,
			buildcmds: buildcmds,
			runcmd:    runcmd,
			status:    a_UPDATING,
			opstatus:  1,
			processes: make(map[uint64]*process, 5),
			plker:     new(sync.RWMutex),
			notice:    make(chan uint64, 100),
		}
		a.logfile, e = rotatefile.NewRotateFile("./app_log/"+fullappname, fullappname)
		if e != nil {
			return errors.New("[CreateApp] " + e.Error())
		}
		a.sloger = slog.New(slog.NewJSONHandler(a.logfile, &slog.HandlerOptions{
			AddSource: true,
			ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
				if len(groups) == 0 && attr.Key == "function" {
					return slog.Attr{}
				}
				if len(groups) == 0 && attr.Key == slog.SourceKey {
					s := attr.Value.Any().(*slog.Source)
					if index := strings.Index(s.File, "corelib@v"); index != -1 {
						s.File = s.File[index:]
					} else if index = strings.Index(s.File, "Corelib@v"); index != -1 {
						s.File = s.File[index:]
					}
				}
				return attr
			}}).WithGroup("msg_kvs"))
		s.apps[fullappname] = a
		go a.startApp()
		return nil
	} else {
		return errors.New("[CreateApp] app already exist")
	}
}
func (s *Super) UpdateApp(project, groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[UpdateApp] app doesn't exist")
	}
	go a.updateApp()
	return nil
}
func (s *Super) BuildApp(project, groupname, appname, commitid string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[BuildApp] app doesn't exist")
	}
	go a.buildApp(commitid)
	return nil
}
func (s *Super) StopApp(project, groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[StopApp] app doesn't exist")
	}
	a.stopApp(false)
	return nil
}
func (s *Super) DeleteApp(project, groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[DeleteApp] app doesn't exist")
	}
	a.stopApp(true)
	return nil
}
func (s *Super) StartAppProcess(project, groupname, appname string, autorestart bool) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[StartAppProcess] app doesn't exist")
	}
	if e := a.startProcess(atomic.AddUint64(&s.processid, 1), autorestart); e != nil {
		return errors.New("[StartAppProcess] " + e.Error())
	}
	return nil
}
func (s *Super) RestartAppProcess(project, groupname, appname string, pid uint64) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[RestartAppProcess] app doesn't exist")
	}
	if e := a.restartProcess(pid); e != nil {
		return errors.New("[RestartAppProcess] " + e.Error())
	}
	return nil
}
func (s *Super) StopAppProcess(project, groupname, appname string, pid uint64) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return errors.New("[StopAppProcess] app doesn't exist")
	}
	a.stopProcess(pid)
	return nil
}

type AppInfo struct {
	Project     string            `json:"project"`
	Group       string            `json:"group"`
	App         string            `json:"app"`
	Url         string            `json:"url"`
	BuildCmds   []*Cmd            `json:"build_cmds"`
	RunCmd      *Cmd              `json:"run_cmd"`
	Status      int32             `json:"status"`    //0 closing,1 updating,2 updatefailed,3 updatesuccess,4 building,5 buildfailed,6 buildsuccess
	OpStatus    int32             `json:"op_status"` //0 idle,1 some operation is happening on this app
	AllBranch   map[string]string `json:"all_branch"`
	AllTag      map[string]string `json:"all_tag"`
	BinCommitid string            `json:"bin_commitid"`
	ProcessInfo []*ProcessInfo    `json:"process_info"`
}
type ProcessInfo struct {
	Lpid        uint64 `json:"logic_pid"`
	Ppid        uint64 `json:"physic_pid"`
	Stime       int64  `json:"start_time"`
	BinCommitid string `json:"bin_commitid"`
	Status      int    `json:"status"` //0 closing,1 starting,2 working
	Restart     int    `json:"restart"`
	AutoRestart bool   `json:"auto_restart"`
}

func (s *Super) GetAppList() map[string]map[string]string {
	s.lker.RLock()
	defer s.lker.RUnlock()
	result := make(map[string]map[string]string)
	for _, a := range s.apps {
		if _, ok := result[a.project]; !ok {
			result[a.project] = make(map[string]string)
		}
		result[a.project][a.group] = a.app
	}
	return result
}
func (s *Super) GetAppInfo(project, groupname, appname string) (*AppInfo, error) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	a, ok := s.apps[project+"."+groupname+"."+appname]
	if !ok {
		return nil, errors.New("[GetAppInfo] app doesn't exist")
	}
	result := new(AppInfo)
	result.Project = a.project
	result.Group = a.group
	result.App = a.app
	result.Url = a.url
	result.BuildCmds = a.buildcmds
	result.RunCmd = a.runcmd
	result.Status = a.status
	result.OpStatus = a.opstatus
	result.AllBranch = a.allbranch
	result.AllTag = a.alltag
	result.BinCommitid = a.bincommitid
	a.plker.RLock()
	defer a.plker.RUnlock()
	result.ProcessInfo = make([]*ProcessInfo, 0, len(a.processes))
	for _, p := range a.processes {
		p.lker.RLock()
		ppid := 0
		if p.status == p_WORKING {
			ppid = p.cmd.Process.Pid
		}
		result.ProcessInfo = append(result.ProcessInfo, &ProcessInfo{
			Lpid:        p.logicpid,
			Ppid:        uint64(ppid),
			Stime:       p.stime,
			BinCommitid: p.bincommitid,
			Status:      p.status,
			Restart:     p.restart,
			AutoRestart: p.autorestart,
		})
		p.lker.RUnlock()
	}
	return result, nil
}
