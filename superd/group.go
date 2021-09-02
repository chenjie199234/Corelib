package superd

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	g_CLOSING = iota

	g_UPDATING
	g_UPDATEFAILED
	g_UPDATESUCCESS

	g_BUILDING
	g_BUILDFAILED
	g_BUILDSUCCESS
)

type group struct {
	s            *Super
	name         string
	url          string
	buildcmds    []*Cmd
	runcmd       *Cmd
	status       int32 //0 closing,1 updating,2 building,3 working
	masterbranch string
	allbranch    []string
	curbranch    string
	binbranch    string
	alltag       []string
	curtag       string
	bintag       string
	curcommitid  string
	bincommitid  string
	opstatus     int32
	processes    map[uint64]*process
	plker        *sync.RWMutex
	logfile      *rotatefile.RotateFile
	notice       chan uint64
}

func (g *group) log(datas ...interface{}) {
	buf := bufpool.GetBuffer()
	buf.Append(time.Now().Format("2006-01-02_15:04:06.000000000"))
	for _, data := range datas {
		buf.Append(" ")
		buf.Append(data)
	}
	buf.Append("\n")
	g.logfile.WriteBuf(buf)
}
func (g *group) startGroup() {
	g.log("start init")
	g.status = g_UPDATING
	if !g.clone() {
		g.status = g_CLOSING
		g.s.notice <- g.name
		g.log("init failed")
		return
	}
	if !g.mastername() {
		g.status = g_CLOSING
		g.s.notice <- g.name
		g.log("init failed")
		return
	}
	if !g.revparse(g.masterbranch, "") {
		g.status = g_CLOSING
		g.s.notice <- g.name
		g.log("init failed")
		return
	}
	atomic.StoreInt32(&g.opstatus, 0)
	g.curbranch = g.masterbranch
	g.curtag = ""
	g.status = g_BUILDFAILED
	g.log("init success")
	go func() {
		defer func() {
			g.logfile.Close()
			g.s.notice <- g.name
		}()
		for {
			select {
			case lpid, ok := <-g.notice:
				if !ok {
					return
				}
				g.plker.Lock()
				delete(g.processes, lpid)
				if g.status == g_CLOSING && len(g.processes) == 0 {
					g.plker.Unlock()
					return
				}
				g.plker.Unlock()
			}
		}
	}()
}
func (g *group) updateGroup() {
	if atomic.SwapInt32(&g.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&g.opstatus, 0)
	if g.status == g_CLOSING {
		return
	}
	g.log("start update")
	oldbranch := g.curbranch
	oldtag := g.curtag
	oldcommitid := g.curcommitid
	oldstatus := g.status
	g.status = g_UPDATING
	//reset all changes
	if !g.cancelmodify() {
		g.status = g_UPDATEFAILED
		g.log("update failed")
		return
	}
	//switch to master
	if !g.checkout("", "") {
		g.status = g_UPDATEFAILED
		g.log("update failed")
		return
	}
	//pull data
	if !g.pull() {
		g.status = g_UPDATEFAILED
		g.log("update failed")
		return
	}
	//switch to origin branch or tag
	//if there is no branch or tag,use master branch
	//if the origin branch or tag not exist,use master branch
	find := false
	if oldtag != "" {
		for _, v := range g.alltag {
			if v == oldtag {
				find = true
				break
			}
		}
	} else if oldbranch != "" {
		for _, v := range g.allbranch {
			if v == oldbranch {
				find = true
				break
			}
		}
	}
	if !find {
		oldbranch = g.masterbranch
		oldtag = ""
	}
	if !g.checkout(oldbranch, oldtag) {
		g.log("update failed")
		return
	}
	if g.curbranch == oldbranch && g.curtag == oldtag && g.curcommitid == oldcommitid {
		g.status = oldstatus
	} else {
		g.status = g_UPDATESUCCESS
	}
	g.log("update success")
}
func (g *group) buildGroup(branch, tag string) {
	if branch == "" && tag == "" {
		g.log("build params empty")
		return
	}
	if atomic.SwapInt32(&g.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&g.opstatus, 0)
	if g.status == g_CLOSING {
		return
	}
	g.log("start build")
	oldstatus := g.status
	oldcommitid := g.curcommitid
	g.status = g_UPDATING
	//reset all changes
	if !g.cancelmodify() {
		g.status = g_UPDATEFAILED
		g.log("build failed")
		return
	}
	//switch to master
	if !g.checkout("", "") {
		g.status = g_UPDATEFAILED
		g.log("build failed")
		return
	}
	//pull data
	if !g.pull() {
		g.status = g_UPDATEFAILED
		g.log("build failed")
		return
	}
	//apply new changes
	if !g.checkout(branch, tag) {
		g.status = g_BUILDFAILED
		g.log("build failed")
		return
	}
	//check changes
	if g.curcommitid == oldcommitid && oldstatus == g_BUILDSUCCESS {
		g.status = g_BUILDSUCCESS
		g.log("build success")
		return
	}
	//build
	if !g.build() {
		g.status = g_BUILDFAILED
		g.log("build failed")
		return
	}
	g.status = g_BUILDSUCCESS
	g.bincommitid = g.curcommitid
	g.binbranch = g.curbranch
	g.bintag = g.curtag
	g.log("build success")
}
func (g *group) stopGroup(del bool) {
	if atomic.SwapInt32(&g.opstatus, 1) == 1 {
		return
	}
	defer atomic.StoreInt32(&g.opstatus, 0)
	if g.status == g_CLOSING {
		return
	}
	g.plker.RLock()
	for _, p := range g.processes {
		go p.stopProcess()
	}
	g.plker.RUnlock()
	if del {
		g.status = g_CLOSING
		if len(g.processes) == 0 {
			close(g.notice)
		}
	}
}
func (g *group) clone() bool {
	cmd := exec.Command("git", "clone", "-q", g.url, "./")
	g.log("start:git clone")
	if !g.git(cmd, "clone") {
		return false
	}
	//update branch and tag data
	wg := sync.WaitGroup{}
	wg.Add(2)
	success := true
	go func() {
		defer wg.Done()
		if !g.branch() {
			success = false
		}
	}()
	go func() {
		defer wg.Done()
		if !g.tag() {
			success = false
		}
	}()
	wg.Wait()
	return success
}
func (g *group) mastername() bool {
	cmd := exec.Command("git", "branch", "--show-current")
	g.log("start:git branch --show-current")
	return g.git(cmd, "mastername")
}
func (g *group) pull() bool {
	cmd := exec.Command("git", "pull", "-q")
	g.log("start:git pull -q")
	if !g.git(cmd, "pull") {
		return false
	}
	//update branch and tag data
	wg := sync.WaitGroup{}
	wg.Add(2)
	success := true
	go func() {
		defer wg.Done()
		if !g.branch() {
			success = false
		}
	}()
	go func() {
		defer wg.Done()
		if !g.tag() {
			success = false
		}
	}()
	wg.Wait()
	return success
}
func (g *group) branch() bool {
	cmd := exec.Command("git", "branch", "-a")
	g.log("start:git branch -a")
	return g.git(cmd, "branch")
}
func (g *group) tag() bool {
	cmd := exec.Command("git", "tag")
	g.log("start:git tag")
	return g.git(cmd, "tag")
}
func (g *group) revparse(branch string, tag string) bool {
	var cmd *exec.Cmd
	if tag != "" {
		cmd = exec.Command("git", "rev-parse", "--short", tag)
		g.log("start:git rev-parse --short", tag)
	} else if branch != "" {
		cmd = exec.Command("git", "rev-parse", "--short", "origin/"+branch)
		g.log("start:git rev-parse --short", "origin/"+branch)
	}
	return g.git(cmd, "revparse")
}
func (g *group) cancelmodify() bool {
	cmd := exec.Command("git", "checkout", "-q", ".")
	g.log("start:git checkout -q .")
	return g.git(cmd, "cancelmodify")
}
func (g *group) checkout(branch string, tag string) bool {
	if branch == "" && tag == "" {
		branch = g.masterbranch
	}
	if !g.revparse(branch, tag) {
		return false
	}
	var cmd *exec.Cmd
	if tag == "" && branch == g.masterbranch {
		cmd = exec.Command("git", "checkout", "-q", g.masterbranch)
		g.log("start:git checkout -q", g.masterbranch)
	} else {
		cmd = exec.Command("git", "checkout", "-q", g.curcommitid)
		g.log("start:git checkout -q", g.curcommitid)
	}
	if !g.git(cmd, "checkout") {
		g.curbranch = ""
		g.curtag = ""
		g.curcommitid = ""
		return false
	}
	if tag != "" {
		g.curtag = tag
		g.curbranch = ""
	} else {
		g.curbranch = branch
		g.curtag = ""
	}
	return true
}
func (g *group) git(cmd *exec.Cmd, operation string) bool {
	cmd.Dir = "./app/" + g.name
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		g.log(operation, "failed:pipe stdout error:", e)
		return false
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		g.log(operation, "failed:pipe stderr error:", e)
		return false
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		g.log(operation, "failed:start process error:", e)
		return false
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		result := make([]string, 0)
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				g.log(operation, "failed:read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			switch operation {
			case "revparse":
				g.curcommitid = common.Byte2str(line)
			case "branch":
				linestr := strings.TrimSpace(common.Byte2str(line))
				if strings.HasPrefix(linestr, "remotes/origin/") && !strings.HasPrefix(linestr, "remotes/origin/HEAD") {
					result = append(result, linestr[15:])
				}
			case "mastername":
				g.masterbranch = common.Byte2str(line)
			case "tag":
				result = append(result, common.Byte2str(line))
			}
			g.log(operation, "stdout:", common.Byte2str(line))
		}
		if operation == "branch" {
			g.allbranch = result
		}
		if operation == "tag" {
			g.alltag = result
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				g.log(operation, "failed:read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			g.log(operation, "stderr:", common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		g.log(operation, "failed:run process error:", e)
		return false
	}
	g.log(operation, "success")
	return true
}
func (g *group) build() bool {
	for _, buildcmd := range g.buildcmds {
		cmd := exec.Command(buildcmd.Cmd, buildcmd.Args...)
		cmd.Env = buildcmd.Env
		cmd.Dir = "./app/" + g.name
		g.log("build start:cmd:", buildcmd.Cmd, "args:", buildcmd.Args, "env:", buildcmd.Env)
		tempout, e := cmd.StdoutPipe()
		if e != nil {
			g.log("build failed:pipe stdout error:", e)
			return false
		}
		temperr, e := cmd.StderrPipe()
		if e != nil {
			g.log("build failed:pipe stderr error:", e)
			return false
		}
		outreader := bufio.NewReaderSize(tempout, 4096)
		errreader := bufio.NewReaderSize(temperr, 4096)
		if e = cmd.Start(); e != nil {
			g.log("build failed:start process error:", e)
			return false
		}
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			for {
				line, _, e := outreader.ReadLine()
				if e != nil && e != io.EOF {
					g.log("build failed:read stdout error:", e)
					break
				} else if e != nil {
					break
				}
				g.log("build stdout:", common.Byte2str(line))
			}
			wg.Done()
		}()
		go func() {
			for {
				line, _, e := errreader.ReadLine()
				if e != nil && e != io.EOF {
					g.log("build failed:read stdout error:", e)
					break
				} else if e != nil {
					break
				}
				g.log("build stderr:", common.Byte2str(line))
			}
			wg.Done()
		}()
		wg.Wait()
		if e = cmd.Wait(); e != nil {
			g.log("build failed:run process error:", e)
			return false
		}
	}
	return true
}
func (g *group) startProcess(pid uint64, autorestart bool) error {
	g.plker.Lock()
	defer g.plker.Unlock()
	p := &process{
		s:           g.s,
		g:           g,
		logicpid:    pid,
		status:      p_STARTING,
		lker:        &sync.RWMutex{},
		init:        true,
		autorestart: autorestart,
	}
	var e error
	p.logfile, e = rotatefile.NewRotateFile("./app_log/"+g.name+"/"+strconv.FormatUint(pid, 10), g.name)
	if e != nil {
		return e
	}
	g.processes[pid] = p
	go p.startProcess()
	return nil
}
func (g *group) restartProcess(pid uint64) error {
	g.plker.RLock()
	defer g.plker.RUnlock()
	p, ok := g.processes[pid]
	if !ok {
		return errors.New("process not exist")
	}
	go p.restartProcess()
	return nil
}
func (g *group) stopProcess(pid uint64) {
	g.plker.RLock()
	defer g.plker.RUnlock()
	if p, ok := g.processes[pid]; ok {
		go p.stopProcess()
	}
}
