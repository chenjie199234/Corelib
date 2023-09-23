package superd

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/rotatefile"
)

const (
	p_CLOSING = iota
	p_STARTING
	p_WORKING
)

type process struct {
	s           *Super
	a           *app
	logicpid    uint64
	status      int //0 closing,1 starting,2 working
	init        bool
	lker        *sync.RWMutex
	autorestart bool
	logfile     *rotatefile.RotateFile
	cmd         *exec.Cmd
	out         *bufio.Reader
	err         *bufio.Reader
	stime       int64
	bincommitid string
	restart     int
}

func (p *process) startProcess() {
	for {
		if p.run() {
			p.output()
			if e := p.cmd.Wait(); e != nil {
				p.a.sloger.Error("run cmd process failed",
					"operation", "process",
					"stime", p.stime,
					"restart", p.restart,
					"commitid", p.bincommitid,
					"logicpid", p.logicpid,
					"cmd", p.a.runcmd.Cmd,
					"args", p.a.runcmd.Args,
					"env", p.a.runcmd.Env)
			} else {
				p.a.sloger.Info("run cmd process success",
					"operation", "process",
					"stime", p.stime,
					"restart", p.restart,
					"commitid", p.bincommitid,
					"logicpid", p.logicpid,
					"cmd", p.a.runcmd.Cmd,
					"args", p.a.runcmd.Args,
					"env", p.a.runcmd.Env)
			}
		}
		p.lker.Lock()
		if !p.autorestart || p.status == p_CLOSING {
			p.lker.Unlock()
			break
		}
		p.status = p_STARTING
		p.lker.Unlock()
		time.Sleep(time.Millisecond * 500)
	}
	p.logfile.Close()
	p.a.notice <- p.logicpid
}
func (p *process) run() bool {
	if atomic.SwapInt32(&p.a.opstatus, 1) == 1 {
		p.a.sloger.Info("app is updating", "operation", "process", "logicpid", p.logicpid)
		return false
	}
	defer atomic.StoreInt32(&p.a.opstatus, 0)
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.a.status == a_CLOSING || p.status == p_CLOSING {
		p.a.sloger.Info("app or process is closing", "operation", "process", "logicpid", p.logicpid)
		return false
	}
	if p.a.bincommitid == "" {
		p.a.sloger.Error("app missing build", "operation", "process", "logicpid", p.logicpid)
		return false
	}
	if p.a.bincommitid != p.bincommitid || p.init {
		//a new build version
		p.restart = 0
		p.init = false
	} else {
		p.restart++
	}
	p.bincommitid = p.a.bincommitid
	p.stime = time.Now().UnixNano()
	p.cmd = exec.Command(p.a.runcmd.Cmd, p.a.runcmd.Args...)
	p.cmd.Env = p.a.runcmd.Env
	p.cmd.Dir = "./app/" + p.a.project + "." + p.a.group + "." + p.a.app
	p.a.sloger.Info("run",
		"operation", "process",
		"stime", p.stime,
		"restart", p.restart,
		"commitid", p.bincommitid,
		"logicpid", p.logicpid,
		"cmd", p.a.runcmd.Cmd,
		"args", p.a.runcmd.Args,
		"env", p.a.runcmd.Env)
	out, e := p.cmd.StdoutPipe()
	if e != nil {
		p.a.sloger.Error("pipe stdout failed",
			"operation", "process",
			"stime", p.stime,
			"restart", p.restart,
			"commitid", p.bincommitid,
			"logicpid", p.logicpid,
			"cmd", p.a.runcmd.Cmd,
			"args", p.a.runcmd.Args,
			"env", p.a.runcmd.Env,
			"error", e)
		return false
	}
	err, e := p.cmd.StderrPipe()
	if e != nil {
		p.a.sloger.Error("pipe stderr failed",
			"operation", "process",
			"stime", p.stime,
			"restart", p.restart,
			"commitid", p.bincommitid,
			"logicpid", p.logicpid,
			"cmd", p.a.runcmd.Cmd,
			"args", p.a.runcmd.Args,
			"env", p.a.runcmd.Env,
			"error", e)
		return false
	}
	p.out = bufio.NewReaderSize(out, 4096)
	p.err = bufio.NewReaderSize(err, 4096)
	if e = p.cmd.Start(); e != nil {
		p.a.sloger.Error("start cmd process failed",
			"operation", "process",
			"stime", p.stime,
			"restart", p.restart,
			"commitid", p.bincommitid,
			"logicpid", p.logicpid,
			"cmd", p.a.runcmd.Cmd,
			"args", p.a.runcmd.Args,
			"env", p.a.runcmd.Env,
			"error", e)
		return false
	}
	p.status = p_WORKING
	return true
}
func (p *process) output() {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for {
			line, _, e := p.out.ReadLine()
			if e != nil && e != io.EOF {
				p.a.sloger.Error("read stdout failed",
					"operation", "process",
					"stime", p.stime,
					"restart", p.restart,
					"commitid", p.bincommitid,
					"logicpid", p.logicpid,
					"cmd", p.a.runcmd.Cmd,
					"args", p.a.runcmd.Args,
					"env", p.a.runcmd.Env,
					"error", e)
				break
			} else if e != nil {
				break
			}
			p.logfile.Write(line)
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := p.err.ReadLine()
			if e != nil && e != io.EOF {
				p.a.sloger.Error("read stderr failed",
					"operation", "process",
					"stime", p.stime,
					"restart", p.restart,
					"commitid", p.bincommitid,
					"logicpid", p.logicpid,
					"cmd", p.a.runcmd.Cmd,
					"args", p.a.runcmd.Args,
					"env", p.a.runcmd.Env,
					"error", e)
				break
			} else if e != nil {
				break
			}
			p.logfile.Write(line)
		}
		wg.Done()
	}()
	wg.Wait()
}
func (p *process) restartProcess() {
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.status == p_WORKING && p.cmd != nil {
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
