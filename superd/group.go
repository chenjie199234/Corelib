package superd

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/rotatefile"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	g_CLOSING = iota
	g_UPDATING
	g_BUILDING
	g_BUILDFAILED
	g_BUILDSUCCESS
)

type group struct {
	s          *Super
	name       string
	url        string
	buildcmds  []*Cmd
	runcmd     *Cmd
	status     int //0 closing,1 updating,2 building,3 working
	allbranch  []string
	branchname string
	alltag     []string
	tagname    string
	commitid   string
	lker       *sync.RWMutex
	processes  map[uint64]*process
	plker      *sync.RWMutex
	logfile    *rotatefile.RotateFile
	notice     chan uint64
}

func (g *group) log(lpid uint64, branch, tag, commitid, logdata string) {
	buf := bufpool.GetBuffer()
	buf.Append("{")
	buf.Append("\"lpid\":")
	buf.Append(lpid)
	if tag != "" {
		buf.Append(",\"tag\":\"")
		buf.Append(tag)
	} else {
		buf.Append(",\"branch\":\"")
		buf.Append(branch)
	}
	buf.Append(",\"commitid\":\"")
	buf.Append(commitid)
	buf.Append("\",\"log\":\"")
	buf.Append(logdata)
	buf.Append("\"}\n")
	g.logfile.WriteBuf(buf)
}
func (g *group) startGroup() {
	log.Info("[group.start] start init group:", g.name)
	g.lker.Lock()
	if !g.clone() {
		g.status = g_CLOSING
		g.s.notice <- g.name
		g.lker.Unlock()
		return
	}
	g.branchname = "master"
	g.status = g_BUILDFAILED
	g.lker.Unlock()
	log.Info("[group.start] init group:", g.name, "success")
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
				log.Info("[group.process] stop process in group:", g.name, "logicpid:", lpid)
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
func (g *group) stopGroup() {
	g.plker.RLock()
	defer g.plker.RUnlock()
	for _, p := range g.processes {
		go p.stopProcess()
	}
}
func (g *group) closeGroup() {
	g.plker.RLock()
	defer g.plker.RUnlock()

	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status == g_CLOSING {
		return
	}
	g.status = g_CLOSING
	if len(g.processes) == 0 {
		close(g.notice)
	} else {
		for _, p := range g.processes {
			go p.stopProcess()
		}
	}
}
func (g *group) clone() bool {
	cmd := exec.Command("git", "clone", "-q", g.url, "./")
	log.Info("[group.clone] group:", g.name, "start:git clone")
	return g.git(cmd, "clone")
}
func (g *group) pull() bool {
	cmd := exec.Command("git", "pull", "-q")
	log.Info("[group.pull] group:", g.name, "start:git pull")
	return g.git(cmd, "pull")
}
func (g *group) checkout(commitid string) bool {
	var cmd *exec.Cmd
	if commitid == "" {
		commitid = "master"
	}
	cmd = exec.Command("git", "checkout", "-q", commitid)
	log.Info("[group.checkout] group:", g.name, "start:git checkout -q", commitid)
	return g.git(cmd, "checkout")
}
func (g *group) revparse(branch string, tag string) bool {
	var cmd *exec.Cmd
	if tag != "" {
		cmd = exec.Command("git", "rev-parse", tag)
		log.Info("[group.revparse] group:", g.name, "start:git rev-parse", tag)
	} else if branch != "" {
		cmd = exec.Command("git", "rev-parse", "origin/"+branch)
		log.Info("[group.revparse] group:", g.name, "start:git rev-parse origin/"+branch)
	}
	return g.git(cmd, "revparse")
}
func (g *group) branch() bool {
	cmd := exec.Command("git", "branch", "-a")
	log.Info("[group.branch] group:", g.name, "start:git branch -a")
	return g.git(cmd, "branch")
}
func (g *group) tag() bool {
	cmd := exec.Command("git", "tag")
	log.Info("[group.tag] group:", g.name, "start:git tag")
	return g.git(cmd, "tag")
}
func (g *group) git(cmd *exec.Cmd, operation string) bool {
	cmd.Dir = "./app/" + g.name
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		log.Error("[group."+operation+"] group:", g.name, "exit error: pipe stdout error:", e)
		return false
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		log.Error("[group."+operation+"] group:", g.name, "exit error: pipe stderr error:", e)
		return false
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		log.Error("[group."+operation+"] group:", g.name, "exit error: start process error:", e)
		return false
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		result := make([]string, 0)
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[group."+operation+"] group:", g.name, "read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			if operation == "revparse" {
				g.commitid = common.Byte2str(line)
			}
			if operation == "branch" {
				linestr := common.Byte2str(line)
				if strings.HasPrefix(linestr, "remotes/origin/") && !strings.HasPrefix(linestr, "remotes/origin/HEAD") {
					result = append(result, linestr[15:])
				}
			}
			if operation == "tag" {
				result = append(result, common.Byte2str(line))
			}
			log.Info("[group."+operation+"] group:", g.name, common.Byte2str(line))
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
				log.Error("[group."+operation+"] group:", g.name, "read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			log.Info("[group."+operation+"] group:", g.name, common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		log.Error("[group."+operation+"] gorup:", g.name, "exit error:", e)
		return false
	}
	log.Info("[group."+operation+"] group:", g.name, "exit success")
	return true
}
func (g *group) build(branchname string, tagname string) {
	g.lker.Lock()
	defer g.lker.Unlock()
	oldstatus := g.status
	g.status = g_UPDATING
	//reset all change
	if !g.checkout(".") {
		g.status = g_BUILDFAILED
		return
	}
	//switch to master
	if !g.checkout("") {
		g.status = g_BUILDFAILED
		return
	}
	//update src
	if !g.pull() {
		g.status = g_BUILDFAILED
		return
	}
	//get new commitid
	oldcommitid := g.commitid
	if tagname != "" {
		if !g.revparse("", tagname) {
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		if g.tagname == tagname && oldcommitid == g.commitid && oldstatus == g_BUILDSUCCESS {
			//don't need to rebuild
			g.status = g_BUILDSUCCESS
			return
		}
	} else if branchname != "" {
		if !g.revparse(branchname, "") {
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		if g.branchname == branchname && oldcommitid == g.commitid && oldstatus == g_BUILDSUCCESS {
			//don't need to rebuild
			g.status = g_BUILDSUCCESS
			return
		}
	} else {
		return
	}
	//switch to new commitid
	if !g.checkout(g.commitid) {
		g.commitid = oldcommitid
		g.status = g_BUILDFAILED
		return
	}
	g.status = g_BUILDING
	for _, buildcmd := range g.buildcmds {
		cmd := exec.Command(buildcmd.Cmd, buildcmd.Args...)
		cmd.Env = buildcmd.Env
		cmd.Dir = "./app/" + g.name
		log.Info("[group.build] group:", g.name, "start:cmd:", buildcmd.Cmd, "args:", buildcmd.Args, "env:", buildcmd.Env)
		tempout, e := cmd.StdoutPipe()
		if e != nil {
			log.Error("[group.build] group:", g.name, "exit error: pipe stdout error:", e)
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		temperr, e := cmd.StderrPipe()
		if e != nil {
			log.Error("[group.build] group:", g.name, "exit error: pipe stderr error:", e)
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		outreader := bufio.NewReaderSize(tempout, 4096)
		errreader := bufio.NewReaderSize(temperr, 4096)
		if e = cmd.Start(); e != nil {
			log.Error("[group.build] group:", g.name, "exit error: start process error:", e)
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			for {
				line, _, e := outreader.ReadLine()
				if e != nil && e != io.EOF {
					log.Error("[group.build] group:", g.name, "read stdout error:", e)
					break
				} else if e != nil {
					break
				}
				log.Info("[group.build] group:", g.name, common.Byte2str(line))
			}
			wg.Done()
		}()
		go func() {
			for {
				line, _, e := errreader.ReadLine()
				if e != nil && e != io.EOF {
					log.Error("[group.build] group:", g.name, "read stderr error:", e)
					break
				} else if e != nil {
					break
				}
				log.Info("[group.build] group:", g.name, common.Byte2str(line))
			}
			wg.Done()
		}()
		wg.Wait()
		if e = cmd.Wait(); e != nil {
			log.Error("[group.build]", g.name, "exit error:", e)
			g.commitid = oldcommitid
			g.status = g_BUILDFAILED
			return
		}
		log.Info("[group.build] group:", g.name, "exit success")
	}
	if tagname != "" {
		g.tagname = tagname
		g.branchname = ""
	} else {
		g.tagname = ""
		g.branchname = branchname
	}
	g.status = g_BUILDSUCCESS
	return
}
func (g *group) startProcess(pid uint64, autorestart bool) {
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
	g.processes[pid] = p
	go p.startProcess()
}
func (g *group) restartProcess(pid uint64) {
	g.plker.RLock()
	defer g.plker.RUnlock()
	if p, ok := g.processes[pid]; ok {
		go p.restartProcess()
	}
}
func (g *group) stopProcess(pid uint64) {
	g.plker.RLock()
	defer g.plker.RUnlock()
	if p, ok := g.processes[pid]; ok {
		go p.stopProcess()
	}
}
