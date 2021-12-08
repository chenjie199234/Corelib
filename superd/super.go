package superd

import (
	"errors"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/name"
)

const (
	s_CLOSING = iota
	s_WORKING
)

//struct
type Super struct {
	processid uint64
	lker      *sync.RWMutex
	groups    map[string]*group
	status    int
	notice    chan string
	closech   chan struct{}
}

//maxlogsize unit M
func NewSuper() *Super {
	instance := &Super{
		lker:    new(sync.RWMutex),
		groups:  make(map[string]*group, 5),
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
			case groupname, ok := <-instance.notice:
				if !ok {
					return
				}
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
	return instance
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
			go g.stopGroup(true)
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

func (s *Super) CreateGroup(groupname, appname, url string, buildcmds []*Cmd, runcmd *Cmd) error {
	//check group name
	fullappname := groupname + "." + appname
	if e := name.FullCheck(fullappname); e != nil {
		return e
	}
	s.lker.Lock()
	defer s.lker.Unlock()
	if s.status == s_CLOSING {
		return errors.New("[create] Superd is closing")
	}
	g, ok := s.groups[fullappname]
	if !ok {
		var e error
		if e = os.RemoveAll("./app_log/" + fullappname); e != nil {
			return errors.New("[create] remove old dir error:" + e.Error())
		}
		if e = os.RemoveAll("./app/" + fullappname); e != nil {
			return errors.New("[create] remote old dir error:" + e.Error())
		}
		if e = dirop("./app_log/" + fullappname); e != nil {
			return errors.New("[create] " + e.Error())
		}
		if e = dirop("./app/" + fullappname); e != nil {
			return errors.New("[create] " + e.Error())
		}
		g = &group{
			s:         s,
			name:      fullappname,
			url:       url,
			buildcmds: buildcmds,
			runcmd:    runcmd,
			status:    g_UPDATING,
			processes: make(map[uint64]*process, 5),
			opstatus:  1,
			plker:     new(sync.RWMutex),
			notice:    make(chan uint64, 100),
		}
		g.logfile, e = rotatefile.NewRotateFile("./app_log/"+fullappname, fullappname)
		if e != nil {
			return errors.New("[create] " + e.Error())
		}
		s.groups[fullappname] = g
		go g.startGroup()
		return nil
	} else {
		return errors.New("[create] group already exist")
	}
}
func (s *Super) UpdateGroup(groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[update] group doesn't exist")
	}
	go g.updateGroup()
	return nil
}
func (s *Super) BuildGroup(groupname, appname, branchname, tagname string) error {
	if branchname == "" && tagname == "" {
		return errors.New("[build] missing branch or tag")
	}
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[build] group doesn't exist")
	}
	go g.buildGroup(branchname, tagname)
	return nil
}
func (s *Super) StopGroup(groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[stop] group doesn't exist")
	}
	g.stopGroup(false)
	return nil
}
func (s *Super) DeleteGroup(groupname, appname string) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[delete] group doesn't exist")
	}
	g.stopGroup(true)
	return nil
}
func (s *Super) StartProcess(groupname, appname string, autorestart bool) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[startprocess] group doesn't exist")
	}
	if e := g.startProcess(atomic.AddUint64(&s.processid, 1), autorestart); e != nil {
		return errors.New("[startprocess] " + e.Error())
	}
	return nil
}
func (s *Super) RestartProcess(groupname, appname string, pid uint64) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[restartprocess] group doesn't exist")
	}
	if e := g.restartProcess(pid); e != nil {
		return errors.New("[restartprocess] " + e.Error())
	}
	return nil
}
func (s *Super) StopProcess(groupname, appname string, pid uint64) error {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return errors.New("[stopprocess] group doesn't exist")
	}
	g.stopProcess(pid)
	return nil
}

type GroupInfo struct {
	Name        string         `json:"name"`
	Url         string         `json:"url"`
	BuildCmds   []*Cmd         `json:"build_cmds"`
	RunCmd      *Cmd           `json:"run_cmd"`
	Status      int32          `json:"status"`    //0 closing,1 updating,2 updatefailed,3 updatesuccess,4 building,5 buildfailed,6 buildsuccess
	OpStatus    int32          `json:"op_status"` //0 idle,1 some operation is happening on this group
	AllBranch   []string       `json:"all_branch"`
	CurBranch   string         `json:"cur_branch"`
	AllTag      []string       `json:"all_tag"`
	CurTag      string         `json:"cur_tag"`
	CurCommitid string         `json:"commitid"`
	Pinfo       []*ProcessInfo `json:"process_info"`
}
type ProcessInfo struct {
	Lpid        uint64 `json:"logic_pid"`
	Ppid        uint64 `json:"physic_pid"`
	Stime       int64  `json:"start_time"`
	BinBranch   string `json:"bin_branch"`
	BinTag      string `json:"bin_tag"`
	BinCommitid string `json:"bin_commitid"`
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
func (s *Super) GetGroupInfo(groupname, appname string) (*GroupInfo, error) {
	s.lker.RLock()
	defer s.lker.RUnlock()
	g, ok := s.groups[groupname+"."+appname]
	if !ok {
		return nil, errors.New("[groupinfo] group doesn't exist")
	}
	result := new(GroupInfo)
	result.Name = g.name
	result.Url = g.url
	result.BuildCmds = g.buildcmds
	result.RunCmd = g.runcmd
	result.Status = g.status
	result.OpStatus = g.opstatus
	result.AllBranch = g.allbranch
	result.CurBranch = g.curbranch
	result.AllTag = g.alltag
	result.CurTag = g.curtag
	result.CurCommitid = g.curcommitid
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
			BinBranch:   p.binbranch,
			BinTag:      p.bintag,
			BinCommitid: p.bincommitid,
			Status:      p.status,
			Restart:     p.restart,
			AutoRestart: p.autorestart,
		})
		p.lker.RUnlock()
	}
	return result, nil
}
