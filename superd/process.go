package superd

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/log"
)

const (
	p_CLOSING = iota
	p_STARTING
	p_WORKING
)

type process struct {
	s            *Super
	g            *group
	logicpid     uint64
	cmd          *exec.Cmd
	out          *bufio.Reader
	err          *bufio.Reader
	lker         *sync.RWMutex
	stime        int64
	status       int //0 closing,1 starting,2 working
	version      string
	versionshort string
	restart      int
	autorestart  bool
}

func (p *process) startProcess() {
	p.restart = -1
	first := true
	for {
		if !first {
			time.Sleep(time.Millisecond * 500)
		}
		first = false
		p.g.lker.RLock()
		if p.g.status == g_CLOSING {
			p.g.lker.RUnlock()
			break
		}
		if p.g.status != g_WORKING {
			p.g.lker.RUnlock()
			continue
		}
		p.lker.Lock()
		if p.status == p_CLOSING {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			break
		}
		if p.g.version != p.version {
			p.restart = 0
		} else {
			p.restart++
		}
		p.version = p.g.version
		if p.g.urlType == UrlGit {
			findex := strings.Index(p.version, "|")
			p.versionshort = p.version[:findex+8]
		} else {
			p.versionshort = p.version
		}
		p.stime = time.Now().UnixNano()
		p.cmd = exec.Command(p.g.runCmd, p.g.runArgs...)
		p.cmd.Env = p.g.runEnv
		p.cmd.Dir = "./app/" + p.g.name
		log.Info("[process.start] start in group:", p.g.name, "on version:", p.version, "logicpid:", p.logicpid)
		out, e := p.cmd.StdoutPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			log.Error("[process.start] in group:", p.g.name, "logicpid:", p.logicpid, "version:", p.version, "pipe stdout error:", e)
			continue
		}
		err, e := p.cmd.StderrPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			log.Error("[process.start] in group:", p.g.name, "logicpid:", p.logicpid, "version:", p.version, "pipe stderr error:", e)
			continue
		}
		p.out = bufio.NewReaderSize(out, 4096)
		p.err = bufio.NewReaderSize(err, 4096)
		if e = p.cmd.Start(); e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			log.Error("[process.start] in group:", p.g.name, "logicpid:", p.logicpid, "version:", p.version, "start new process error:", e)
			continue
		}
		p.status = p_WORKING
		p.lker.Unlock()
		p.g.lker.RUnlock()
		p.log()
		if e = p.cmd.Wait(); e != nil {
			log.Error("[process.start] in group:", p.g.name, "logicpid:", p.logicpid, "version:", p.version, "exit process error:", e)
		} else {
			log.Info("[process.start] in group:", p.g.name, "logicpid:", p.logicpid, "version:", p.version, "exit process success")
		}
		p.lker.Lock()
		if p.autorestart && p.status == p_WORKING {
			p.status = p_STARTING
		} else {
			p.status = p_CLOSING
		}
		p.lker.Unlock()
	}
	p.g.notice <- p.logicpid
}
func (p *process) log() {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := p.out.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[process.log] in group:", p.g.name, "logicpid:", p.logicpid, "physicpid:", p.cmd.Process.Pid, "version:", p.version, "read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			p.g.log(p.logicpid, uint64(p.cmd.Process.Pid), p.versionshort, common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := p.err.ReadLine()
			if e != nil && e != io.EOF {
				log.Error("[process.log] in group:", p.g.name, "logicpid:", p.logicpid, "physicpid:", p.cmd.Process.Pid, "version:", p.version, "read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			p.g.log(p.logicpid, uint64(p.cmd.Process.Pid), p.versionshort, common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
}
func (p *process) restartProcess() {
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.status == p_WORKING && p.cmd != nil {
		p.restart = -1
		p.cmd.Process.Signal(syscall.SIGTERM)
	}
}
func (p *process) stopProcess() {
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.status == p_CLOSING {
		return
	}
	p.status = p_CLOSING
	if p.cmd != nil {
		p.cmd.Process.Signal(syscall.SIGTERM)
	}
}
