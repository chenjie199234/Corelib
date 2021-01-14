package superd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/rotatefile"
)

const (
	g_CLOSING = iota
	g_UPDATING
	g_BUILDING
	g_WORKING
)
const (
	g_URLTYPE_GIT = iota
	g_URLTYPE_BIN
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
	lf, ok := g.s.pool.Get().(*logformat)
	if !ok {
		lf = &logformat{
			Superd: g.s.name,
		}
	}
	lf.Group = g.name
	lf.Lpid = lpid
	lf.Ppid = ppid
	lf.Version = version
	lf.Log = logdata
	g.loglker.Lock()
	now := time.Now()
	lf.Time = fmt.Sprintf(now.Format("2006-01-02 15:04:05")+".%9d ", now.Nanosecond())
	d, _ := json.Marshal(lf)
	g.logfile.Write(append(d, '\n'))
	g.loglker.Unlock()
	g.s.pool.Put(lf)
}
func (g *group) startGroup() {
	g.lker.Lock()
	if g.status != g_UPDATING {
		g.s.notice <- g.name
		g.lker.Unlock()
		return
	}
	switch g.urlType {
	case g_URLTYPE_BIN:
		if e := g.bin(); e != nil {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
	case g_URLTYPE_GIT:
		if e := g.gitclone(); e != nil {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
		if g.buildCmd != "" {
			if e := g.build(); e != nil {
				g.status = g_CLOSING
				g.s.notice <- g.name
				g.lker.Unlock()
				return
			}
		}
		if e := g.gitinfo(); e != nil {
			g.status = g_CLOSING
			g.s.notice <- g.name
			g.lker.Unlock()
			return
		}
	}
	g.status = g_WORKING
	g.lker.Unlock()

	defer func() {
		g.logfile.Close(true)
		g.s.notice <- g.name
	}()
	for {
		select {
		case lpid, ok := <-g.notice:
			if !ok {
				return
			}
			g.lker.Lock()
			delete(g.processes, lpid)
			if g.status == g_CLOSING && len(g.processes) == 0 {
				g.lker.Unlock()
				return
			}
			g.lker.Unlock()
		}
	}
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
			p.stopProcess()
		}
	}
}
func (g *group) updateGroupSrc() {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status != g_WORKING {
		g.logs("[update group]group status can't update git source now")
		return
	}
	g.status = g_UPDATING
	defer func() {
		g.status = g_WORKING
	}()
	switch g.urlType {
	case g_URLTYPE_BIN:
		if e := g.bin(); e != nil {
			return
		}
	case g_URLTYPE_GIT:
		if e := g.gitpull(); e != nil {
			return
		}
		if g.buildCmd != "" {
			if e := g.build(); e != nil {
				return
			}
		}
		if e := g.gitinfo(); e != nil {
			return
		}
	}
	return
}
func (g *group) switchGroupBranch(branch string) {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.urlType != g_URLTYPE_GIT {
		g.logs("[update group]group isn't a git based group")
		return
	}
	if g.status != g_WORKING {
		g.logs("[update group]group status can't switch git branch now")
		return
	}
	vers := strings.Split(g.version, "|")
	if len(vers) > 0 && vers[0] == branch {
		return
	}
	g.status = g_UPDATING
	defer func() {
		g.status = g_WORKING
	}()
	if e := g.checkout(branch); e != nil {
		return
	}
	if e := g.gitpull(); e != nil {
		return
	}
	if g.buildCmd != "" {
		if e := g.build(); e != nil {
			return
		}
	}
	if e := g.gitinfo(); e != nil {
		return
	}
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
func (g *group) bin() error {
	f, e := os.OpenFile("./app/"+g.name+"/"+g.name+"_temp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0744)
	if e != nil {
		e = fmt.Errorf("[download bin]write file open error:%s", e)
		g.logs(e.Error())
		return e
	}
	resp, e := http.Get(g.url)
	if e != nil {
		e = fmt.Errorf("[download bin]http request error:%s", e)
		g.logs(e.Error())
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e = fmt.Errorf("[download bin]http response status code:%d msg:%s", resp.StatusCode, resp.Status)
		g.logs(e.Error())
		return e
	}
	if _, e = io.Copy(f, resp.Body); e != nil {
		e = fmt.Errorf("[download bin]copy data into bin file error%s", e)
		g.logs(e.Error())
		return e
	}
	if e = os.Rename("./app/"+g.name+"/"+g.name+"_temp", "./app/"+g.name+"/"+g.name); e != nil {
		e = fmt.Errorf("[download bin]cover old bin file with new bin file error:%s", e)
		g.logs(e.Error())
		return e
	}
	g.version = resp.Header.Get("Version")
	g.logs("[download bin]exit success")
	return nil
}
func (g *group) gitclone() error {
	cmd := exec.Command("git", "clone", "-q", g.url, "./")
	g.logs(fmt.Sprintf("[git clone]start clone from:%s", g.url))
	return g.git(cmd, "git clone")
}
func (g *group) gitpull() error {
	cmd := exec.Command("git", "pull", "-q")
	g.logs(fmt.Sprintf("[git pull]start pull from:%s", g.url))
	return g.git(cmd, "git pull")
}
func (g *group) checkout(branch string) error {
	cmd := exec.Command("git", "checkout", "-q", branch)
	g.logs(fmt.Sprintf("[git checkout]start checkout to branch:%s", branch))
	return g.git(cmd, "git checkout")
}
func (g *group) git(cmd *exec.Cmd, operation string) error {
	cmd.Dir = "./app/" + g.name
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		e = fmt.Errorf("[%s]pipe stdout error:%s", operation, e)
		g.logs(e.Error())
		return e
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		e = fmt.Errorf("[%s]pipe stderr error:%s", operation, e)
		g.logs(e.Error())
		return e
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		e = fmt.Errorf("[%s]start process error:%s", operation, e)
		g.logs(e.Error())
		return e
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[%s]read stdout error:%s", operation, e))
				break
			} else if e != nil {
				break
			}
			g.logs(fmt.Sprintf("[%s]%s", operation, line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[%s]read stderr error:%s", operation, e))
				break
			} else if e != nil {
				break
			}
			g.logs(fmt.Sprintf("[%s]%s", operation, line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		e = fmt.Errorf("[%s]exit error:%s", operation, e)
		g.logs(e.Error())
		return e
	} else {
		g.logs(fmt.Sprintf("[%s]exit success", operation))
	}
	return nil
}
func (g *group) gitinfo() error {
	cmd := exec.Command("git", "log", "-1", "--pretty=format:\"%D|%h|%cn(%ce)|%cI\"")
	cmd.Dir = "./app/" + g.name
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		e = fmt.Errorf("[git info]pipe stdout error:%s", e)
		g.logs(e.Error())
		return e
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		e = fmt.Errorf("[git info]pipe stderr error:%s", e)
		g.logs(e.Error())
		return e
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		e = fmt.Errorf("[git info]start process error:%s", e)
		g.logs(e.Error())
		return e
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[git info]read stdout error:%s", e))
				break
			} else if e != nil {
				break
			}
			tempstr := common.Byte2str(line)
			index := strings.Index(tempstr, "|")
			branch := tempstr[:index]
			rest := tempstr[index : len(tempstr)-1]
			tempstrs := strings.Split(branch, ",")
			tempstr = tempstrs[0]
			tempstrs = strings.Split(tempstr, "->")
			branch = strings.TrimSpace(tempstrs[1])
			g.version = branch + rest
			g.logs(fmt.Sprintf("[git info]%s", line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[git info]read stderr error:%s", e))
				break
			} else if e != nil {
				break
			}
			g.logs(fmt.Sprintf("[git info]%s", line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		e = fmt.Errorf("[git info]exit error:%s", e)
		g.logs(e.Error())
		return e
	} else {
		g.logs("[get info]exit success")
	}
	return nil
}

func (g *group) build() error {
	cmd := exec.Command(g.buildCmd, g.buildArgs...)
	cmd.Env = g.buildEnv
	cmd.Dir = "./app/" + g.name
	cmd.Stderr = os.Stdout
	tempout, e := cmd.StdoutPipe()
	if e != nil {
		e = fmt.Errorf("[build]pipe stdout error:%s", e)
		g.logs(e.Error())
		return e
	}
	temperr, e := cmd.StderrPipe()
	if e != nil {
		e = fmt.Errorf("[build]pipe stdout error:%s", e)
		g.logs(e.Error())
		return e
	}
	outreader := bufio.NewReaderSize(tempout, 4096)
	errreader := bufio.NewReaderSize(temperr, 4096)
	if e = cmd.Start(); e != nil {
		e = fmt.Errorf("[build]start process error:%s", e)
		g.logs(e.Error())
		return e
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := outreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[build]read stdout error:%s", e))
				break
			} else if e != nil {
				break
			}
			g.logs(fmt.Sprintf("[build]%s", line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := errreader.ReadLine()
			if e != nil && e != io.EOF {
				g.logs(fmt.Sprintf("[build]read stderr error:%s", e))
				break
			} else if e != nil {
				break
			}
			g.logs(fmt.Sprintf("[build]%s", line))
		}
		wg.Done()
	}()
	wg.Wait()
	if e = cmd.Wait(); e != nil {
		e = fmt.Errorf("[build]exit error:%s", e)
		g.logs(e.Error())
		return e
	} else {
		g.logs("[build]exit success")
	}
	return nil
}
func (g *group) startProcess(pid uint64) bool {
	g.lker.Lock()
	defer g.lker.Unlock()
	if g.status != g_WORKING {
		return false
	}
	p := &process{
		s:        g.s,
		g:        g,
		logicpid: pid,
		status:   p_STARTING,
		lker:     &sync.RWMutex{},
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
	p.restartProcess()
}
func (g *group) stopProcess(pid uint64) {
	g.lker.Lock()
	defer g.lker.Unlock()
	p, ok := g.processes[pid]
	if !ok {
		return
	}
	p.stopProcess()
}
func (g *group) logs(logdata string) {
	g.s.log(g.name, 0, 0, "", logdata)
}
