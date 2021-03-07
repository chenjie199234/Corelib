package superd

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	p_CLOSING = iota
	p_STARTING
	p_WORKING
)

type process struct {
	s           *Super
	g           *group
	logicpid    uint64
	cmd         *exec.Cmd
	out         *bufio.Reader
	err         *bufio.Reader
	lker        *sync.RWMutex
	stime       int64
	status      int //0 closing,1 starting,2 working
	branchname  string
	tagname     string
	commitid    string
	restart     int
	init        bool
	autorestart bool
}

func (p *process) startProcess() {
	for {
		p.g.lker.RLock()
		if p.g.status == g_CLOSING {
			p.g.lker.RUnlock()
			break
		}
		if p.g.status == g_BUILDFAILED {
			p.g.lker.RUnlock()
			p.restart++
			log.Error("[process.start] start in group:", p.g.name, "error:last build is failed")
			if p.autorestart {
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
		p.lker.Lock()
		if p.status == p_CLOSING {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			break
		}
		if p.g.branchname != p.branchname || p.g.tagname != p.tagname || p.g.commitid != p.commitid || p.init {
			//a new build version
			p.restart = 0
			p.init = false
		} else {
			p.restart++
		}

		p.branchname = p.g.branchname
		p.tagname = p.g.tagname
		p.commitid = p.g.commitid
		p.stime = time.Now().UnixNano()
		p.cmd = exec.Command(p.g.runcmd.Cmd, p.g.runcmd.Args...)
		p.cmd.Env = p.g.runcmd.Env
		p.cmd.Dir = "./app/" + p.g.name
		if p.tagname != "" {
			log.Info("[process.start] start in group:", p.g.name, "tag:", p.tagname, "commitid:", p.commitid, "logicpid:", p.logicpid)
		} else {
			log.Info("[process.start] start in group:", p.g.name, "branch:", p.branchname, "commitid:", p.commitid, "logicpid:", p.logicpid)
		}
		out, e := p.cmd.StdoutPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			if p.tagname != "" {
				log.Error("[process.start] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "pipe stdout error:", e)
			} else {
				log.Error("[process.start] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "pipe stdout error:", e)
			}
			if p.autorestart {
				time.Sleep(time.Millisecond * 50)
				continue
			} else {
				break
			}
		}
		err, e := p.cmd.StderrPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			if p.tagname != "" {
				log.Error("[process.start] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "pipe stderr error:", e)
			} else {
				log.Error("[process.start] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "pipe stderr error:", e)
			}
			if p.autorestart {
				time.Sleep(time.Millisecond * 50)
				continue
			} else {
				break
			}
		}
		p.out = bufio.NewReaderSize(out, 4096)
		p.err = bufio.NewReaderSize(err, 4096)
		if e = p.cmd.Start(); e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			if p.tagname != "" {
				log.Error("[process.start] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "start new process error:", e)
			} else {
				log.Error("[process.start] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "start new process error:", e)
			}
			if p.autorestart {
				time.Sleep(time.Millisecond * 50)
				continue
			} else {
				break
			}
		}
		p.status = p_WORKING
		p.lker.Unlock()
		p.g.lker.RUnlock()
		p.log()
		if e = p.cmd.Wait(); e != nil {
			if p.tagname != "" {
				log.Error("[process.start] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "exit process error:", e)
			} else {
				log.Error("[process.start] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "exit process error:", e)
			}
		} else {
			if p.tagname != "" {
				log.Info("[process.start] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "exit process success")
			} else {
				log.Info("[process.start] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "exit process success")

			}
		}
		p.lker.Lock()
		if p.autorestart && p.status == p_WORKING {
			p.status = p_STARTING
			p.lker.Unlock()
			time.Sleep(time.Millisecond * 50)
		} else {
			p.status = p_CLOSING
			p.lker.Unlock()
		}
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
				if p.tagname != "" {
					log.Error("[process.log] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "read stdout error:", e)
				} else {
					log.Error("[process.log] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "read stdout error:", e)
				}
				break
			} else if e != nil {
				break
			}
			p.g.log(p.logicpid, p.branchname, p.tagname, p.commitid[:8], common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := p.err.ReadLine()
			if e != nil && e != io.EOF {
				if p.tagname != "" {
					log.Error("[process.log] in group:", p.g.name, "tag:", p.tagname, "commitid", p.commitid, "logicpid:", p.logicpid, "read stderr error:", e)
				} else {
					log.Error("[process.log] in group:", p.g.name, "branch:", p.branchname, "commitid", p.commitid, "logicpid:", p.logicpid, "read stderr error:", e)
				}
				break
			} else if e != nil {
				break
			}
			p.g.log(p.logicpid, p.branchname, p.tagname, p.commitid[:8], common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
}
func (p *process) restartProcess() {
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.status == p_WORKING && p.cmd != nil {
		p.restart = 0
		p.init = true
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
