package superd

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chenjie199234/Corelib/rotatefile"
)

//struct
type super struct {
	name       string
	processid  uint64
	workdir    string
	lker       *sync.Mutex
	all        map[string]*group
	loglker    *sync.Mutex
	logfile    *rotatefile.RotateFile
	maxlogsize uint64
}
type group struct {
	name     string
	lker     *sync.Mutex
	Cmd      string              `json:"cmd"`
	Args     []string            `json:"args"`
	All      map[uint64]*process `json:"all"`
	Required int                 `json:"required"`
	loglker  *sync.Mutex
	logfile  *rotatefile.RotateFile
}
type process struct {
	Stime  int64 `json:"start_time"`
	Killed bool  `json:"killed"`
	cmd    *exec.Cmd
	out    *bufio.Reader
}

var instance *super

//logic
func init() {
	tempname := flag.String("name", "", "specify the superd's name,default:super")
	tempdir := flag.String("workdir", "", "specify the work dir,default:current dir will be used.")
	templogsize := flag.Uint64("maxlogsize", 0, "specify the max log file size,unit is M,default:100M.")
	flag.Parse()
	if *tempname == "" {
		*tempname = "super"
	}
	if *tempdir == "" {
		*tempdir = "./"
	}
	if *templogsize == 0 {
		*templogsize = 100 * 1024 * 1024
	}
	instance = &super{
		processid:  0,
		name:       *tempname,
		workdir:    *tempdir,
		lker:       new(sync.Mutex),
		all:        make(map[string]*group, 5),
		loglker:    new(sync.Mutex),
		maxlogsize: *templogsize,
	}
	//workdir
	var e error
	if e = workdirop(); e != nil {
		panic("[init]" + e.Error())
	}
	//bin dir
	if e = dirop("./bin"); e != nil {
		panic("[init]" + e.Error())
	}
	//self log dir
	if e = dirop("./self_log"); e != nil {
		panic("[init]" + e.Error())
	}
	//process log dir
	if e = dirop("./process_log"); e != nil {
		panic("[init]" + e.Error())
	}
	//self log file
	instance.logfile, e = rotatefile.NewRotateFile("./self_log", instance.name, instance.maxlogsize)
	if e != nil {
		panic("[init]" + e.Error())
	}
	//env
	pathenv := os.Getenv("PATH")
	if pathenv == "" {
		pathenv = instance.workdir + "/bin"
	} else {
		pathenv += ":" + instance.workdir + "/bin"
	}
	os.Setenv("PATH", pathenv)
}
func workdirop() error {
	temp, e := filepath.Abs(instance.workdir)
	if e != nil {
		return fmt.Errorf("change config workdir:%s to abs path error:%s", instance.workdir, e)
	}
	instance.workdir = temp
	finfo, e := os.Lstat(instance.workdir)
	if e != nil && os.IsNotExist(e) {
		if e = os.MkdirAll(instance.workdir+"/bin", 0755); e != nil {
			return fmt.Errorf("workdir:%s not exist,and create error:%s", instance.workdir, e)
		}
	} else if e != nil {
		return fmt.Errorf("get workdir:%s info error:%s", instance.workdir, e)
	} else if !finfo.IsDir() {
		return fmt.Errorf("workdir:%s exist and is not a dir", instance.workdir)
	}
	if e = os.Chdir(instance.workdir); e != nil {
		return fmt.Errorf("enter workdir:%s error:%s", instance.workdir, e)
	}
	return nil
}
func dirop(path string) error {
	finfo, e := os.Lstat(path)
	if e != nil && os.IsNotExist(e) {
		if e = os.MkdirAll(path, 0755); e != nil {
			return fmt.Errorf("%s dir in workdir:%s not exist,and create error:%s", path, instance.workdir, e)
		}
	} else if e != nil {
		return fmt.Errorf("get %s dir info in workdir:%s error:%s", path, instance.workdir, e)
	} else if !finfo.IsDir() {
		return fmt.Errorf("%s in workdir:%s is not a dir", path, instance.workdir)
	}
	return nil
}
func getInfo() []byte {
	d, _ := json.Marshal(instance.all)
	return d
}
func createGroup(name string, cmd string, args ...string) error {
	if len(name) == 0 || name[0] < 65 || (name[0] > 90 && name[0] < 97) || name[0] > 122 {
		return fmt.Errorf("[create group]group name illegal,must start with [a-z][A-Z]")
	}
	for _, v := range name {
		if (v < 65 && v != 46) || (v > 90 && v < 97) || v > 122 {
			return fmt.Errorf("[create group]group name has illegal character,only support[a-z][A-Z][.]")
		}
	}
	//path, e := exec.LookPath(cmd)
	//if e != nil {
	//        return fmt.Errorf("[create group]cmd can't be found")
	//}
	//finfo, e := os.Stat(path)
	//if e != nil {
	//        return fmt.Errorf("[create group]get cmd path info error:%s", e)
	//}
	//if finfo.IsDir() {
	//        return fmt.Errorf("[create group]cmd path is a dir")
	//}
	instance.lker.Lock()
	defer instance.lker.Unlock()
	_, ok := instance.all[name]
	if !ok {
		e := dirop("./process_log/" + name)
		if e != nil {
			return fmt.Errorf("[create group]" + e.Error())
		}
		temp := &group{
			name:     name,
			lker:     new(sync.Mutex),
			Cmd:      cmd,
			Args:     args,
			All:      make(map[uint64]*process, 5),
			Required: 0,
			loglker:  new(sync.Mutex),
		}
		temp.logfile, e = rotatefile.NewRotateFile("./process_log/"+name, name, instance.maxlogsize)
		if e != nil {
			return fmt.Errorf("[create group]" + e.Error())
		}
		instance.all[name] = temp
	}
	return nil
}
func setProcess(name string, required int) error {
	instance.lker.Lock()
	defer instance.lker.Unlock()
	group, ok := instance.all[name]
	if !ok {
		return fmt.Errorf("[set group]group doesn't exist,please create the group before set process num.")
	}
	group.lker.Lock()
	defer group.lker.Unlock()
	if group.Required > required {
		//shirnk,random stop process
		for i := 0; i < group.Required-required; i++ {
			for _, p := range group.All {
				p.Killed = true
				p.cmd.Process.Signal(syscall.SIGTERM)
			}
		}
		group.Required = required
	} else if group.Required < required {
		//grow
		for i := 0; i < required-group.Required; i++ {
			pid := atomic.AddUint64(&instance.processid, 1)
			tempprocess := &process{}
			group.All[pid] = tempprocess
			go runProcess(group, pid, tempprocess, group.Cmd, group.Args)
		}
		group.Required = required
	}
	return nil
}
func runProcess(g *group, pid uint64, p *process, cmd string, args []string) {
	for {
		//first check
		if p.Killed {
			break
		}
		p.Stime = time.Now().UnixNano()
		p.cmd = exec.Command(cmd, args...)
		p.cmd.Stderr = p.cmd.Stdout
		out, e := p.cmd.StdoutPipe()
		if e != nil {
			//TODO log self
			continue
		}
		p.out = bufio.NewReaderSize(out, 4096)
		if e = p.cmd.Start(); e != nil {
			//TODO log self
			continue
		}
		//double check
		if p.Killed {
			p.cmd.Process.Signal(syscall.SIGTERM)
			break
		}
		go log(g, p.out, pid, uint64(p.cmd.Process.Pid))
		if e = p.cmd.Wait(); e != nil {
			//TODO log self
		} else {
			//TODO log self
		}
	}
	g.lker.Lock()
	defer g.lker.Unlock()
	delete(g.All, pid)
}
func log(g *group, reader *bufio.Reader, logicpid uint64, physicpid uint64) {
	for {
		line, _, e := reader.ReadLine()
		if e != nil && e == io.EOF {
			return
		}
		if e != nil {
			//TODO log self
			continue
		}
		//TODO log process
	}
}
func stopProcess(name string, pid uint64) {
	instance.lker.Lock()
	defer instance.lker.Unlock()
	group, ok := instance.all[name]
	if !ok {
		return
	}
	group.lker.Lock()
	defer group.lker.Unlock()
	p, ok := group.All[pid]
	if !ok {
		return
	}
	if !p.Killed {
		p.Killed = true
		p.cmd.Process.Signal(syscall.SIGTERM)
	}
}
func deleteGroup(name string) error {
	instance.lker.Lock()
	defer instance.lker.Unlock()
	group, ok := instance.all[name]
	if !ok {
		return nil
	}
	group.lker.Lock()
	defer group.lker.Unlock()
	if group.Required != 0 {
		return fmt.Errorf("[delete group]there are processes running in this group,please stop all processes before delete the group.")
	}
	delete(instance.all, name)
	return nil
}
