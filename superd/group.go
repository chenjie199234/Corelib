package superd

import (
	"bufio"
	"io"
	"io/ioutil"
	"net/http"
	"os"
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
	g_WORKING
)

type group struct {
	s         *Super
	name      string
	url       string
	urlType   int
	buildCmd  string
	buildArgs []string
	buildEnv  []string
	runCmd    string
	runArgs   []string
	runEnv    []string
	status    int //0 closing,1 updating,2 building,3 working
	version   string
	processes map[uint64]*process
	lker      *sync.RWMutex
	logfile   *rotatefile.RotateFile
	loglker   *sync.Mutex
	notice    chan uint64
}

func (g *group) log(lpid, ppid uint64, version, logdata string) {
	buf := bufpool.GetBuffer()
	buf.Append("{")
	buf.Append("\"lpid\":")
	buf.Append(lpid)
	buf.Append(",\"ppid\":")
	buf.Append(ppid)
	buf.Append(",\"ver\":\"")
	buf.Append(version)
	buf.Append("\",\"log\":\"")
	buf.Append(logdata)
	buf.Append("\"}\n")
	g.logfile.WriteBuf(buf)
}
func (g *group) startGroup() {
	log.Info("[group.start]", g.name, "init")
	g.lker.Lock()
	switch g.urlType {
	case UrlBin:
		if !g.bin() {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
	case UrlGit:
		if !g.gitclone() {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
		if g.buildCmd != "" && !g.build() {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
		if !g.gitinfo() {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
	}
	g.status = g_WORKING
	g.lker.Unlock()
	log.Info("[group.start]", g.name, "init success")
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
				log.Info("[group.process]", g.name, "stop process logicpid:", lpid)
				g.lker.Lock()
				delete(g.processes, lpid)
				if g.status == g_CLOSING && len(g.processes) == 0 {
					g.lker.Unlock()
					return
				}
				g.lker.Unlock()
			}
		}
	}()
}
func (g *group) stopGroup() {
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
func (g *group) updateGroupSrc() {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status != g_WORKING {
		log.Error("[group.update]", g.name, "status:", g.status, "can't update bin/src")
		return
	}
	log.Info("[group.update]", g.name, "start")
	g.status = g_UPDATING
	defer func() {
		g.status = g_WORKING
	}()
	switch g.urlType {
	case UrlBin:
		if !g.bin() {
			return
		}
	case UrlGit:
		if !g.gitpull() {
			return
		}
		if g.buildCmd != "" && !g.build() {
			return
		}
		if !g.gitinfo() {
			return
		}
	}
	log.Info("[group.update]", g.name, "exit success")
	return
}
func (g *group) switchGroupBranch(branch string) {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.urlType != UrlGit {
		log.Error("[group.switch]", g.name, "isn't a git based group")
		return
	}
	if g.status != g_WORKING {
		log.Error("[group.switch]", g.name, "status:", g.status, "can't switch branch")
		return
	}
	vers := strings.Split(g.version, "|")
	if len(vers) > 0 && vers[0] == branch {
		return
	}
	log.Info("[group.switch]", g.name, "start")
	g.status = g_UPDATING
	defer func() {
		g.status = g_WORKING
	}()
	if !g.checkout(branch) {
		return
	}
	if !g.gitpull() {
		return
	}
	if g.buildCmd != "" && !g.build() {
		return
	}
	if !g.gitinfo() {
		return
	}
	log.Info("[group.switch]", g.name, "exit success")
	return
}
func (g *group) deleteGroup() bool {
	g.lker.Lock()
	defer g.lker.Unlock()
	if len(g.processes) > 0 {
		return false
	}
	g.status = g_CLOSING
	close(g.notice)
	return true
}
func (g *group) bin() bool {
	log.Info("[group.bin]", g.name, "start")
	f, e := os.OpenFile("./app/"+g.name+"/"+g.name+"_temp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0744)
	if e != nil {
		log.Error("[group.bin]", g.name, "exit error: create temp file error:", e)
		return false
	}
	resp, e := http.Get(g.url)
	if e != nil {
		log.Error("[group.bin]", g.name, "exit error: http request error:", e)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if data, e := ioutil.ReadAll(resp.Body); e != nil {
			log.Error("[group.bin]", g.name, "exit error: http response error: code:", resp.StatusCode, "msg:read msg error:", e)
		} else {
			log.Error("[group.bin]", g.name, "exit error: http response error: code:", resp.StatusCode, "msg:", common.Byte2str(data))
		}
		return false
	}
	if _, e = io.Copy(f, resp.Body); e != nil {
		log.Error("[group.bin]", g.name, "exit error: copy data into bin file error:", e)
		return false
	}
	if e = os.Rename("./app/"+g.name+"/"+g.name+"_temp", "./app/"+g.name+"/"+g.name); e != nil {
		log.Error("[group.bin]", g.name, "exit error: cover old bin file with new bin file error:", e)
		return false
	}
	g.version = resp.Header.Get("Version")
	log.Info("[group.bin]", g.name, "exit success")
	return true
}
func (g *group) gitclone() bool {
	cmd := exec.Command("git", "clone", "-q", g.url, "./")
	log.Info("[group.clone]", g.name, "start")
	return g.git(cmd, "clone")
}
func (g *group) gitpull() bool {
	cmd := exec.Command("git", "pull", "-q")
	log.Info("[group.pull]", g.name, "start")
	return g.git(cmd, "pull")
}
func (g *group) checkout(branch string) bool {
	cmd := exec.Command("git", "checkout", "-q", branch)
	log.Info("[group.checkout]", g.name, "start")
	return g.git(cmd, "checkout")
}
func (g *group) gitinfo() bool {
	cmd := exec.Command("git", "log", "-1", "--pretty=format:\"%D|%h|%cn(%ce)|%cI\"")
	log.Info("[group.info]", g.name, "start")
	return g.git(cmd, "info")
}
func (g *group) git(cmd *exec.Cmd, operation string) bool {
	cmd.Dir = "./app/" + g.name
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		log.Error("[group."+operation+"]", g.name, "exit error: pipe stdout error:", e)
		return false
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		log.Error("[group."+operation+"]", g.name, "exit error: pipe stderr error:", e)
		return false
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		log.Error("[group."+operation+"]", g.name, "exit error: start process error:", e)
		return false
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[group."+operation+"]", g.name, "read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			if operation == "info" {
				tempstr := common.Byte2str(line)
				index := strings.Index(tempstr, "|")
				branch := tempstr[:index]
				rest := tempstr[index : len(tempstr)-1]
				tempstrs := strings.Split(branch, ",")
				tempstr = tempstrs[0]
				tempstrs = strings.Split(tempstr, "->")
				branch = strings.TrimSpace(tempstrs[1])
				g.version = branch + rest
			}
			log.Info("[group."+operation+"]", g.name, common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[group."+operation+"]", g.name, "read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			log.Info("[group."+operation+"]", g.name, common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		log.Error("[group."+operation+"]", g.name, "exit error:", e)
		return false
	}
	log.Info("[group."+operation+"]", g.name, "exit success")
	return true
}

func (g *group) build() bool {
	g.status = g_BUILDING
	cmd := exec.Command(g.buildCmd, g.buildArgs...)
	cmd.Env = g.buildEnv
	cmd.Dir = "./app/" + g.name
	log.Info("[group.build]", g.name, "start")
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		log.Error("[group.build]", g.name, "exit error: pipe stdout error:", e)
		return false
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		log.Error("[group.build]", g.name, "exit error: pipe stderr error:", e)
		return false
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		log.Error("[group.build]", g.name, "exit error: start process error:", e)
		return false
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[group.build]", g.name, "read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			log.Info("[group.build]", g.name, common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[group.build]", g.name, "read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			log.Info("[group.build]", g.name, common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		log.Error("[group.build]", g.name, "exit error:", e)
		return false
	}
	log.Info("[group.build]", g.name, "exit success")
	return true
}
func (g *group) startProcess(pid uint64, autorestart bool) bool {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status != g_WORKING {
		return false
	}
	p := &process{
		s:           g.s,
		g:           g,
		logicpid:    pid,
		status:      p_STARTING,
		lker:        &sync.RWMutex{},
		autorestart: autorestart,
	}
	g.processes[pid] = p
	go p.startProcess()
	return true
}
func (g *group) restartProcess(pid uint64) {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status != g_WORKING {
		return
	}
	p, ok := g.processes[pid]
	if !ok {
		return
	}
	go p.restartProcess()
}
func (g *group) stopProcess(pid uint64) {
	g.lker.Lock()
	defer g.lker.Unlock()
	p, ok := g.processes[pid]
	if !ok {
		return
	}
	go p.stopProcess()
}
