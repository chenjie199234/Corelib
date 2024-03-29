package superd

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	a_CLOSING = iota

	a_UPDATING
	a_UPDATEFAILED
	a_UPDATESUCCESS

	a_BUILDING
	a_BUILDFAILED
	a_BUILDSUCCESS
)

type app struct {
	s           *Super
	project     string
	group       string
	app         string
	url         string
	buildcmds   []*Cmd
	runcmd      *Cmd
	status      int32             //0 closing,1 updating,2 building,3 working
	allbranch   map[string]string //key branch name,value commitid
	alltag      map[string]string //key tag name,value commitid
	bincommitid string
	opstatus    int32
	processes   map[uint64]*process
	plker       *sync.RWMutex
	logfile     *rotatefile.RotateFile
	sloger      *slog.Logger
	notice      chan uint64
}

func (a *app) startApp() {
	a.sloger.Info("start init")
	a.status = a_UPDATING
	if !a.clone() {
		a.status = a_CLOSING
		a.s.notice <- a.project + "." + a.group + "." + a.app
		a.sloger.Error("init failed")
		return
	}
	atomic.StoreInt32(&a.opstatus, 0)
	a.status = a_UPDATESUCCESS
	a.sloger.Info("init success")
	go func() {
		defer func() {
			a.logfile.Close()
			a.s.notice <- a.project + "." + a.group + "." + a.app
		}()
		for {
			select {
			case lpid, ok := <-a.notice:
				if !ok {
					return
				}
				a.plker.Lock()
				delete(a.processes, lpid)
				if a.status == a_CLOSING && len(a.processes) == 0 {
					a.plker.Unlock()
					return
				}
				a.plker.Unlock()
			}
		}
	}()
}
func (a *app) updateApp() {
	if atomic.SwapInt32(&a.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&a.opstatus, 0)
	if a.status == a_CLOSING {
		return
	}
	a.sloger.Info("start update")
	a.status = a_UPDATING
	//reset all changes
	if !a.cancelmodify() {
		a.status = a_UPDATEFAILED
		a.sloger.Error("update failed")
		return
	}
	//fetch data
	if !a.fetch() {
		a.status = a_UPDATEFAILED
		a.sloger.Error("update failed")
		return
	}
	a.status = a_UPDATESUCCESS
	a.sloger.Info("update success")
}
func (a *app) buildApp(commitid string) {
	if atomic.SwapInt32(&a.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&a.opstatus, 0)
	if a.status == a_CLOSING {
		return
	}
	a.sloger.Info("start build")
	if a.bincommitid == commitid {
		a.sloger.Info("build success")
		return
	}
	a.status = a_UPDATING
	//reset all changes
	if !a.cancelmodify() {
		a.status = a_UPDATEFAILED
		a.sloger.Error("build failed", "commitid", commitid)
		return
	}
	//switch to new changes
	if !a.checkout(commitid) {
		a.status = a_UPDATEFAILED
		a.sloger.Error("build failed", "commitid", commitid)
		return
	}
	//build
	if !a.build() {
		a.status = a_BUILDFAILED
		a.sloger.Error("build failed", "commitid", commitid)
		return
	}
	a.status = a_BUILDSUCCESS
	a.bincommitid = commitid
	a.sloger.Info("build success", "commitid", commitid)
}
func (a *app) stopApp(del bool) {
	if atomic.SwapInt32(&a.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&a.opstatus, 0)
	if a.status == a_CLOSING {
		return
	}
	a.plker.RLock()
	for _, p := range a.processes {
		go p.stopProcess()
	}
	a.plker.RUnlock()
	if del {
		a.status = a_CLOSING
		if len(a.processes) == 0 {
			close(a.notice)
		}
	}
}

// git clone -q <url> ./
func (a *app) clone() bool {
	cmd := exec.Command("git", "clone", "-q", a.url, "./")
	a.sloger.Info("start:git clone -q "+a.url, "operation", "clone")
	if !a.git(cmd, "clone") {
		return false
	}
	//update branch and tag data
	wg := sync.WaitGroup{}
	wg.Add(2)
	success := true
	go func() {
		defer wg.Done()
		if !a.branch() {
			success = false
		}
	}()
	go func() {
		defer wg.Done()
		if !a.tag() {
			success = false
		}
	}()
	wg.Wait()
	return success
}

// git fetch -q
func (a *app) fetch() bool {
	cmd := exec.Command("git", "fetch", "-q")
	a.sloger.Info("start:git fetch -q", "operation", "fetch")
	if !a.git(cmd, "fetch") {
		return false
	}
	//update branch and tag data
	wg := sync.WaitGroup{}
	wg.Add(2)
	success := true
	go func() {
		defer wg.Done()
		if !a.branch() {
			success = false
		}
	}()
	go func() {
		defer wg.Done()
		if !a.tag() {
			success = false
		}
	}()
	wg.Wait()
	return success
}

// git branch -r --format="%(refname:strip=3) %(objectname:short)"
func (a *app) branch() bool {
	cmd := exec.Command("git", "branch", "-r", "--format=\"%(refname:strip=3) %(objectname:short)\"")
	a.sloger.Info("start:git branch -r --format=\"%(refname:strip=3) %(objectname:short)\"", "operation", "listbranch")
	return a.git(cmd, "listbranch")
}

// git tag --format="%(refname:strip=2) %(objectname:short)"
func (a *app) tag() bool {
	cmd := exec.Command("git", "tag", "--format=\"%(refname:strip=2) %(objectname:short)\"")
	a.sloger.Info("start:git tag --format=\"%(refname:strip=2) %(objectname:short)\"", "operation", "listtag")
	return a.git(cmd, "listtag")
}

// git checkout -q .
func (a *app) cancelmodify() bool {
	cmd := exec.Command("git", "checkout", "-q", ".")
	a.sloger.Info("start:git checkout -q .", "operation", "cancelmodify")
	return a.git(cmd, "cancelmodify")
}

// git checkout -q <commitid>
func (a *app) checkout(commitid string) bool {
	cmd := exec.Command("git", "checkout", "-q", commitid)
	a.sloger.Info("start:git checkout -q "+commitid, "operation", "checkout")
	return a.git(cmd, "checkout")
}
func (a *app) git(cmd *exec.Cmd, operation string) bool {
	cmd.Dir = "./app/" + a.project + "." + a.group + "." + a.app
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		a.sloger.Error("pipe stdout failed", "operation", operation, "error", e)
		return false
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		a.sloger.Error("pipe stderr failed", "operation", operation, "error", e)
		return false
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		a.sloger.Error("start cmd process failed", "operation", operation, "error", e)
		return false
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		result := make(map[string]string)
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				a.sloger.Error("read stdout failed", "operation", operation, "error", e)
				break
			} else if e != nil {
				break
			}
			switch operation {
			case "listbranch":
				pieces := strings.Split(strings.TrimSpace(common.BTS(line)), " ")
				if pieces[0] != "HEAD" {
					result[pieces[0]] = pieces[1]
				}
			case "listtag":
				pieces := strings.Split(strings.TrimSpace(common.BTS(line)), " ")
				result[pieces[0]] = pieces[1]
			}
			a.sloger.Info("stdout:"+common.BTS(line), "operation", operation)
		}
		if operation == "listbranch" {
			a.allbranch = result
		}
		if operation == "listtag" {
			a.alltag = result
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				a.sloger.Error("read stderr failed", "operation", operation, "error", e)
				break
			} else if e != nil {
				break
			}
			a.sloger.Error("stderr:"+common.BTS(line), "operation", operation)
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		a.sloger.Error("run cmd process failed", "operation", operation, "error", e)
		return false
	}
	a.sloger.Info("success", "operation", operation)
	return true
}
func (a *app) build() bool {
	for _, buildcmd := range a.buildcmds {
		cmd := exec.Command(buildcmd.Cmd, buildcmd.Args...)
		cmd.Env = buildcmd.Env
		cmd.Dir = "./app/" + a.project + "." + a.group + "." + a.app
		a.sloger.Info("start:build", "cmd", buildcmd.Cmd, "args", buildcmd.Args, "env", buildcmd.Env)
		tempout, e := cmd.StdoutPipe()
		if e != nil {
			a.sloger.Error("pipe stdout failed", "operation", "build", "error", e)
			return false
		}
		temperr, e := cmd.StderrPipe()
		if e != nil {
			a.sloger.Error("pipe stderr failed", "operation", "build", "error", e)
			return false
		}
		outreader := bufio.NewReaderSize(tempout, 4096)
		errreader := bufio.NewReaderSize(temperr, 4096)
		if e = cmd.Start(); e != nil {
			a.sloger.Error("start cmd process failed", "operation", "build", "error", e)
			return false
		}
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			for {
				line, _, e := outreader.ReadLine()
				if e != nil && e != io.EOF {
					a.sloger.Error("read stdout failed", "operation", "build", "error", e)
					break
				} else if e != nil {
					break
				}
				a.sloger.Info("stdout:"+common.BTS(line), "operation", "build")
			}
			wg.Done()
		}()
		go func() {
			for {
				line, _, e := errreader.ReadLine()
				if e != nil && e != io.EOF {
					a.sloger.Error("read stdout failed", "operation", "build", "error", e)
					break
				} else if e != nil {
					break
				}
				a.sloger.Error("stderr:"+common.BTS(line), "operation", "build")
			}
			wg.Done()
		}()
		wg.Wait()
		if e = cmd.Wait(); e != nil {
			a.sloger.Error("run cmd process failed", "operation", "build", "error", e)
			return false
		}
	}
	return true
}
func (a *app) startProcess(pid uint64, autorestart bool) error {
	a.plker.Lock()
	defer a.plker.Unlock()
	if a.bincommitid == "" {
		return errors.New("missing build")
	}
	p := &process{
		s:           a.s,
		a:           a,
		logicpid:    pid,
		status:      p_STARTING,
		init:        true,
		lker:        &sync.RWMutex{},
		autorestart: autorestart,
	}
	var e error
	p.logfile, e = rotatefile.NewRotateFile("./app_log/"+a.project+"."+a.group+"."+a.app+"/"+strconv.FormatUint(pid, 10), a.project+"."+a.group+"."+a.app)
	if e != nil {
		return e
	}
	a.processes[pid] = p
	go p.startProcess()
	return nil
}
func (g *app) restartProcess(pid uint64) error {
	g.plker.RLock()
	defer g.plker.RUnlock()
	p, ok := g.processes[pid]
	if !ok {
		return errors.New("process not exist")
	}
	go p.restartProcess()
	return nil
}
func (g *app) stopProcess(pid uint64) {
	g.plker.RLock()
	defer g.plker.RUnlock()
	if p, ok := g.processes[pid]; ok {
		go p.stopProcess()
	}
}
