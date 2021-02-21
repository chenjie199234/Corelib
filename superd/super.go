package superd

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	s_CLOSING = iota
	s_WORKING
)
const (
	UrlGit = iota
	UrlBin
)

//struct
type Super struct {
	processid uint64
	name      string
	lker      *sync.RWMutex
	groups    map[string]*group
	logsize   uint64
	logcycle  uint64
	status    int
	notice    chan string
	closech   chan struct{}
}

//maxlogsize unit M
func NewSuper(supername string, rotatelogcap rotatefile.RotateCap, rotatelogcycle rotatefile.RotateTime) (*Super, error) {
	if supername == "" {
		supername = "super"
	}
	instance := &Super{
		name:     supername,
		lker:     new(sync.RWMutex),
		groups:   make(map[string]*group, 5),
		logsize:  uint64(rotatelogcap),
		logcycle: uint64(rotatelogcycle),
		status:   s_WORKING,
		notice:   make(chan string, 100),
		closech:  make(chan struct{}, 1),
	}
	var e error
	//app dir
	if e = dirop("./app"); e != nil {
		return nil, e
	}
	//app stdout and stderr log dir
	if e = dirop("./app_log"); e != nil {
		return nil, e
	}
	go func() {
		defer func() {
			instance.closech <- struct{}{}
		}()
		for {
			select {
			case groupname, ok := <-instance.notice:
				if !ok {
					return
				}
				log.Info("[super.group] stop group name:", groupname)
				instance.lker.Lock()
				delete(instance.groups, groupname)
				if instance.status == s_CLOSING && len(instance.groups) == 0 {
					instance.lker.Unlock()
					return
				}
				instance.lker.Unlock()
			}
		}
	}()
	return instance, nil
}
func (s *Super) CloseSuper() {
	s.lker.Lock()
	if s.status == s_CLOSING {
		s.lker.Unlock()
		return
	}
	s.status = s_CLOSING
	if len(s.groups) == 0 {
		close(s.notice)
	} else {
		for _, g := range s.groups {
			go g.stopGroup()
		}
	}
	s.lker.Unlock()
	<-s.closech
	return
}
func dirop(path string) error {
	finfo, e := os.Lstat(path)
	if e != nil && os.IsNotExist(e) {
		if e = os.MkdirAll(path, 0755); e != nil {
			return fmt.Errorf("%s dir not exist,and create error:%s", path, e)
		}
	} else if e != nil {
		return fmt.Errorf("get %s dir info error:%s", path, e)
	} else if !finfo.IsDir() {
		return fmt.Errorf("%s is not a dir", path)
	}
	return nil
}
func (s *Super) CreateGroup(groupname, url string, urltype int, buildcmd string, buildargs, buildenv []string, runcmd string, runargs, runenv []string, logkeepdays uint64) error {
	//check group name
	if e := common.NameCheck(groupname, true); e != nil {
		return fmt.Errorf("[create group]" + e.Error())
	}
	for _, v := range groupname {
		if (v < 65 && v != 46) || (v > 90 && v < 97) || v > 122 {
			return fmt.Errorf("[create group]group name has illegal character,only support[a-z][A-Z][.]")
		}
	}
	if urltype != UrlBin && urltype != UrlGit {
		return fmt.Errorf("[create group]unsupported url type")
	}
	s.lker.Lock()
	defer s.lker.Unlock()
	if s.status == s_CLOSING {
		return fmt.Errorf("[create group]superd is closing")
	}
	g, ok := s.groups[groupname]
	if !ok {
		var e error
		if e = os.RemoveAll("./app_log"); e != nil {
			return fmt.Errorf("[create group] remove old dir error:%s", e)
		}
		if e = os.RemoveAll("./app/" + groupname); e != nil {
			return fmt.Errorf("[create group]remote old dir error:%s", e)
		}
		if e = dirop("./app_log/" + groupname); e != nil {
			return fmt.Errorf("[create group]%s", e)
		}
		if e = dirop("./app/" + groupname); e != nil {
			return fmt.Errorf("[create group]%s", e)
		}
		g = &group{
			s:         s,
			name:      groupname,
			url:       url,
			urlType:   urltype,
			buildCmd:  buildcmd,
			buildArgs: buildargs,
			buildEnv:  buildenv,
			runCmd:    runcmd,
			runArgs:   runargs,
			runEnv:    runenv,
			status:    g_UPDATING,
			processes: make(map[uint64]*process, 5),
			lker:      new(sync.RWMutex),
			loglker:   new(sync.Mutex),
			notice:    make(chan uint64, 100),
		}
		g.logfile, e = rotatefile.NewRotateFile("./app_log/"+groupname, groupname, "log", rotatefile.RotateCap(s.logsize), rotatefile.RotateTime(s.logcycle), rotatefile.KeepDays(logkeepdays))
		if e != nil {
			return fmt.Errorf("[create group]" + e.Error())
		}
		s.groups[groupname] = g
		go g.startGroup()
		return nil
	} else {
		return fmt.Errorf("[create group]group already exist")
	}
}
func (s *Super) DeleteGroup(groupname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return nil
	}
	if !g.deleteGroup() {
		return fmt.Errorf("[delete group]There are processes in this group,please stop them first.")
	}
	return nil
}
func (s *Super) StartProcess(groupname string, autorestart bool) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return fmt.Errorf("[start process]Group doesn't exist,please create the group before start process.")
	}
	go g.startProcess(atomic.AddUint64(&s.processid, 1), autorestart)
	return nil
}
func (s *Super) RestartProcess(groupname string, pid uint64) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return
	}
	go g.restartProcess(pid)
}
func (s *Super) StopProcess(groupname string, pid uint64) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return
	}
	go g.stopProcess(pid)
}
func (s *Super) UpdateGroupSrc(groupname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return fmt.Errorf("[update group src]Group doesn't exist,please create the group before update src.")
	}
	go g.updateGroupSrc()
	return nil
}
func (s *Super) SwitchGroupBranch(groupname string, branch string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return fmt.Errorf("[update group src]Group doesn't exist,please create the group before switch branch.")
	}
	go g.switchGroupBranch(branch)
	return nil
}

type GroupInfo struct {
	Name      string         `json:"name"`
	Url       string         `json:"url"`
	UrlType   int            `json:"url_type"`
	BuildCmd  string         `json:"build_cmd"`
	BuildArgs []string       `json:"build_args"`
	BuildEnv  []string       `json:"build_env"`
	RunCmd    string         `json:"run_cmd"`
	RunArgs   []string       `json:"run_args"`
	RunEnv    []string       `json:"run_env"`
	Status    int            `json:"status"` //0 closing,1 updating,2 building,3 working
	Version   string         `json:"version"`
	Pinfo     []*ProcessInfo `json:"process_info"`
}
type ProcessInfo struct {
	Lpid        uint64 `json:"logic_pid"`
	Ppid        uint64 `json:"physic_pid"`
	Stime       int64  `json:"start_time"`
	Version     string `json:"version"`
	Status      int    `json:"status"` //0 closing,1 starting,2 working
	Restart     int    `json:"restart"`
	AutoRestart bool   `json:"auto_restart"`
}

func (s *Super) GetInfo() []*GroupInfo {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if s.status == s_CLOSING {
		return nil
	}
	result := make([]*GroupInfo, len(s.groups))
	gindex := 0
	for _, g := range s.groups {
		g.lker.RLock()
		tempg := &GroupInfo{
			Name:      g.name,
			Url:       g.url,
			UrlType:   g.urlType,
			BuildCmd:  g.buildCmd,
			BuildArgs: g.buildArgs,
			BuildEnv:  g.buildEnv,
			RunCmd:    g.runCmd,
			RunArgs:   g.runArgs,
			RunEnv:    g.runEnv,
			Status:    g.status,
			Version:   g.version,
			Pinfo:     make([]*ProcessInfo, len(g.processes)),
		}
		pindex := 0
		for _, p := range g.processes {
			p.lker.RLock()
			tempp := &ProcessInfo{
				Lpid:        p.logicpid,
				Stime:       p.stime,
				Version:     p.version,
				Status:      p.status,
				Restart:     p.restart,
				AutoRestart: p.autorestart,
			}
			if p.status == p_WORKING {
				tempp.Ppid = uint64(p.cmd.Process.Pid)
			}
			p.lker.RUnlock()
			tempg.Pinfo[pindex] = tempp
			pindex++
		}
		g.lker.RUnlock()
		sort.Slice(tempg.Pinfo, func(i, j int) bool {
			return tempg.Pinfo[i].Lpid < tempg.Pinfo[j].Lpid
		})
		result[gindex] = tempg
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	//d, _ := json.Marshal(result)
	return result
}
