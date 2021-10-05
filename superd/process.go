package superd

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/bufpool"
	"github.com/chenjie199234/Corelib/rotatefile"
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
	binbranch   string
	bintag      string
	bincommitid string
	restart     int
	init        bool
	logfile     *rotatefile.RotateFile
	autorestart bool
}

func (p *process) log(datas ...interface{}) {
	buf := bufpool.GetBuffer()
	buf.AppendStdTime(time.Now())
	for _, data := range datas {
		buf.AppendByte(' ')
		writeany(buf, data)
	}
	buf.AppendByte('\n')
	p.logfile.WriteBuf(buf)
}
func (p *process) startProcess() {
	for {
		//if !p.run() {
		//break
		//}
		if p.bintag != "" {
			p.log("[start] start with tag:", p.bintag, "commitid:", p.bincommitid, "logicpid:", p.logicpid, "restart:", p.restart, "cmd:", p.g.runcmd.Cmd, "args:", p.g.runcmd.Args, "env:", p.g.runcmd.Env)
		} else {
			p.log("[start] start with branch:", p.binbranch, "commitid:", p.bincommitid, "logicpid:", p.logicpid, "restart", p.restart, "cmd:", p.g.runcmd.Cmd, "args:", p.g.runcmd.Args, "env:", p.g.runcmd.Env)
		}
		if p.run() {
			p.output()
			if e := p.cmd.Wait(); e != nil {
				p.log("[start] exit process error:", e)
			} else {
				p.log("[start] exir process success")
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
	p.g.notice <- p.logicpid
}
func (p *process) run() bool {
	if atomic.SwapInt32(&p.g.opstatus, 1) == 1 {
		p.log("[run] can't run now,group is updating")
		return false
	}
	defer atomic.StoreInt32(&p.g.opstatus, 0)
	p.lker.Lock()
	defer p.lker.Unlock()
	if p.g.status == g_CLOSING || p.status == p_CLOSING {
		p.log("[run] can't run now,group or process is closing")
		return false
	}
	if p.g.status == g_BUILDFAILED || p.g.bincommitid == "" {
		p.log("[run] can't run now,last build failed")
		return true
	}
	if p.g.binbranch != p.binbranch || p.g.bintag != p.bintag || p.g.bincommitid != p.bincommitid || p.init {
		//a new build version
		p.restart = 0
		p.init = false
	} else {
		p.restart++
	}
	p.binbranch = p.g.binbranch
	p.bintag = p.g.bintag
	p.bincommitid = p.g.bincommitid
	p.stime = time.Now().UnixNano()
	p.cmd = exec.Command(p.g.runcmd.Cmd, p.g.runcmd.Args...)
	p.cmd.Env = p.g.runcmd.Env
	p.cmd.Dir = "./app/" + p.g.name
	out, e := p.cmd.StdoutPipe()
	if e != nil {
		p.log("[run] pipe stdout error:", e)
		return false
	}
	err, e := p.cmd.StderrPipe()
	if e != nil {
		p.log("[run] pipe stderr error:", e)
		return false
	}
	p.out = bufio.NewReaderSize(out, 4096)
	p.err = bufio.NewReaderSize(err, 4096)
	if e = p.cmd.Start(); e != nil {
		p.log("[run] start new process error:", e)
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
				p.log("[output] read stdout error:", e)
				break
			} else if e != nil {
				break
			}
			p.log("[output] ", common.Byte2str(line))
		}
		wg.Done()
	}()
	go func() {
		for {
			line, _, e := p.err.ReadLine()
			if e != nil && e != io.EOF {
				p.log("[output] read stderr error:", e)
				break
			} else if e != nil {
				break
			}
			p.log("[output] ", common.Byte2str(line))
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
