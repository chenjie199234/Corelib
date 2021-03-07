package superd

import (
	"errors"
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

//struct
type Super struct {
	c         *GroupConfig
	processid uint64
	lker      *sync.RWMutex
	groups    map[string]*group
	status    int
	notice    chan string
	closech   chan struct{}
}
type GroupConfig struct {
	SuperName      string
	LogRotateCap   uint //unit M
	LogRotateCycle uint //0-off,1-hour,2-day,3-week,4-month
	LogKeepDays    uint //unit day
}

//maxlogsize unit M
func NewSuper(c *GroupConfig) (*Super, error) {
	if c.SuperName == "" {
		c.SuperName = "super"
	}
	instance := &Super{
		c:       c,
		lker:    new(sync.RWMutex),
		groups:  make(map[string]*group, 5),
		status:  s_WORKING,
		notice:  make(chan string, 100),
		closech: make(chan struct{}, 1),
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
				log.Info("[super.group] stop group:", groupname)
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
			go g.closeGroup()
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
			return errors.New(path + " dir not exist,and create error:" + e.Error())
		}
	} else if e != nil {
		return errors.New("get " + path + " dir info error:" + e.Error())
	} else if !finfo.IsDir() {
		return errors.New(path + " is not a dir")
	}
	return nil
}

type Cmd struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`
	Env  []string `json:"env"`
}

func (s *Super) CreateGroup(groupname, url string, buildcmds []*Cmd, runcmd *Cmd, logkeepdays uint64) error {
	//check group name
	if e := common.NameCheck(groupname, true); e != nil {
		return errors.New("[create group]" + e.Error())
	}
	s.lker.Lock()
	defer s.lker.Unlock()
	if s.status == s_CLOSING {
		return errors.New("[create group]Superd is closing")
	}
	g, ok := s.groups[groupname]
	if !ok {
		var e error
		if e = os.RemoveAll("./app_log"); e != nil {
			return errors.New("[create group] remove old dir error:" + e.Error())
		}
		if e = os.RemoveAll("./app/" + groupname); e != nil {
			return errors.New("[create group]remote old dir error:" + e.Error())
		}
		if e = dirop("./app_log/" + groupname); e != nil {
			return errors.New("[create group]" + e.Error())
		}
		if e = dirop("./app/" + groupname); e != nil {
			return errors.New("[create group]" + e.Error())
		}
		g = &group{
			s:         s,
			name:      groupname,
			url:       url,
			buildcmds: buildcmds,
			runcmd:    runcmd,
			status:    g_UPDATING,
			processes: make(map[uint64]*process, 5),
			lker:      new(sync.RWMutex),
			plker:     new(sync.RWMutex),
			notice:    make(chan uint64, 100),
		}
		g.logfile, e = rotatefile.NewRotateFile(&rotatefile.Config{
			Path:        "./app_log/" + groupname,
			Name:        groupname,
			Ext:         "log",
			RotateCap:   s.c.LogRotateCap,
			RotateCycle: s.c.LogRotateCycle,
			KeepDays:    s.c.LogKeepDays,
		})
		if e != nil {
			return errors.New("[create group]" + e.Error())
		}
		s.groups[groupname] = g
		go g.startGroup()
		return nil
	} else {
		return errors.New("[create group]Group already exist")
	}
}
func (s *Super) BuildGroup(groupname, branchname, tagname string) error {
	if branchname == "" && tagname == "" {
		return errors.New("[build group]Missing branch or tag.")
	}
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return errors.New("[build group]Group doesn't exist.")
	}
	go g.build(branchname, tagname)
	return nil
}
func (s *Super) StopGroup(groupname string) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if g, ok := s.groups[groupname]; ok {
		go g.stopGroup()
	}
}
func (s *Super) DeleteGroup(groupname string) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if g, ok := s.groups[groupname]; ok {
		go g.closeGroup()
	}
}
func (s *Super) StartProcess(groupname string, autorestart bool) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if g, ok := s.groups[groupname]; ok {
		go g.startProcess(atomic.AddUint64(&s.processid, 1), autorestart)
	}
}
func (s *Super) RestartProcess(groupname string, pid uint64) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if g, ok := s.groups[groupname]; ok {
		go g.restartProcess(pid)
	}
}
func (s *Super) StopProcess(groupname string, pid uint64) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	if g, ok := s.groups[groupname]; ok {
		go g.stopProcess(pid)
	}
}

type GroupInfo struct {
	Name       string         `json:"name"`
	Url        string         `json:"url"`
	BuildCmds  []*Cmd         `json:"build_cmds"`
	RunCmd     *Cmd           `json:"run_cmd"`
	AllBranch  []string       `json:"all_branch"`
	BranchName string         `json:"branch_name"`
	AllTag     []string       `json:"all_tag"`
	TagName    string         `json:"tag_name"`
	Commitid   string         `json:"commitid"`
	Status     int            `json:"status"` //0 closing,1 updating,2 building,3 working
	Pinfo      []*ProcessInfo `json:"process_info"`
}
type ProcessInfo struct {
	Lpid        uint64 `json:"logic_pid"`
	Ppid        uint64 `json:"physic_pid"`
	Stime       int64  `json:"start_time"`
	BranchName  string `json:"branch_name"`
	TagName     string `json:"tag_name"`
	Commitid    string `json:"commitid"`
	Status      int    `json:"status"` //0 closing,1 starting,2 working
	Restart     int    `json:"restart"`
	AutoRestart bool   `json:"auto_restart"`
}

func (s *Super) GetGroupList() []string {
	s.lker.RLock()
	defer s.lker.RUnlock()
	result := make([]string, 0, len(s.groups)+1)
	for k := range s.groups {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
func (s *Super) GetGroupInfo(groupname string) (*GroupInfo, error) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname]
	if !ok {
		return nil, errors.New("[group info]Group doesn't exist.")
	}
	result := new(GroupInfo)
	result.Name = g.name
	result.Url = g.url
	result.BuildCmds = g.buildcmds
	result.RunCmd = g.runcmd
	result.Status = g.status
	result.BranchName = g.branchname
	result.TagName = g.tagname
	result.Commitid = g.commitid
	result.AllBranch = g.allbranch
	result.AllTag = g.alltag
	g.plker.RLock()
	defer g.plker.RUnlock()
	result.Pinfo = make([]*ProcessInfo, 0, len(g.processes))
	for _, p := range g.processes {
		p.lker.RLock()
		ppid := 0
		if p.status == p_WORKING {
			ppid = p.cmd.Process.Pid
		}
		result.Pinfo = append(result.Pinfo, &ProcessInfo{
			Lpid:        p.logicpid,
			Ppid:        uint64(ppid),
			Stime:       p.stime,
			BranchName:  p.branchname,
			TagName:     p.tagname,
			Commitid:    p.commitid,
			Status:      p.status,
			Restart:     p.restart,
			AutoRestart: p.autorestart,
		})
		p.lker.RUnlock()
	}
	return result, nil
}
