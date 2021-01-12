package superd

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/common"
)

const (
	p_CLOSING = iota
	p_STARTING
	p_WORKING
)

type process struct {
	s        *Super
	g        *group
	logicpid uint64
	cmd      *exec.Cmd
	out      *bufio.Reader
	err      *bufio.Reader
	lker     *sync.RWMutex
	stime    int64
	status   int //0 closing,1 starting,2 working
	version  string
}

func (p *process) startProcess() {
	first := true
	for {
		if !first {
			time.Sleep(time.Millisecond * 50)
		}
		first = false
		p.g.lker.RLock()
		if p.g.status == g_CLOSING {
			//group is closing
			p.g.lker.RUnlock()
			break
		}
		if p.g.status != g_WORKING {
			//this is impossible,group status must be working
			p.g.lker.RUnlock()
			continue
		}
		p.lker.Lock()
		if p.status == p_CLOSING {
			//process status must be starting
			p.lker.Unlock()
			p.g.lker.RUnlock()
			break
		}
		p.version = p.g.version
		p.stime = time.Now().UnixNano()
		p.cmd = exec.Command(p.g.runCmd, p.g.runArgs...)
		p.cmd.Env = p.g.runEnv
		p.cmd.Dir = "./app/" + p.g.name
		p.logs("begin start")
		out, e := p.cmd.StdoutPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			p.logs(fmt.Sprintf("pipe stdout error:%s", e))
			continue
		}
		err, e := p.cmd.StderrPipe()
		if e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			p.logs(fmt.Sprintf("pipe stderr error:%s", e))
			continue
		}
		p.out = bufio.NewReaderSize(out, 4096)
		p.err = bufio.NewReaderSize(err, 4096)
		if e = p.cmd.Start(); e != nil {
			p.lker.Unlock()
			p.g.lker.RUnlock()
			p.logs(fmt.Sprintf("start new process error:%s", e))
			continue
		}
		p.status = p_WORKING
		p.lker.Unlock()
		p.g.lker.RUnlock()
		p.log()
		if e = p.cmd.Wait(); e != nil {
			p.logs(fmt.Sprintf("exit error:%s", e))
		} else {
			p.logs("exit success")
		}
		p.lker.Lock()
		if p.status == p_WORKING {
			p.status = p_STARTING
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
				p.logs(fmt.Sprintf("read stdout error:%s", e))
				break
			} else if e != nil {
				break
			}
			p.logg(common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := p.err.ReadLine()
			if e != nil && e != io.EOF {
				p.logs(fmt.Sprintf("read stderr error:%s", e))
				break
			} else if e != nil {
				break
			}
			p.logg(common.Byte2str(line))
		}
		wg.Done()
	}()
	wg.Wait()
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
func (p *process) logs(data string) {
	if p.cmd == nil || p.cmd.Process == nil {
		p.s.log(p.g.name, p.logicpid, 0, p.version, data)
	} else {
		p.s.log(p.g.name, p.logicpid, uint64(p.cmd.Process.Pid), p.version, data)
	}
}
func (p *process) logg(data string) {
	if p.cmd == nil || p.cmd.Process == nil {
		p.g.log(p.logicpid, 0, p.version, data)
	} else {
		p.g.log(p.logicpid, uint64(p.cmd.Process.Pid), p.version, data)
	}
}
