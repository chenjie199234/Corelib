package discovery

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRSINIT    = fmt.Errorf("[Discovery.server]not init,call NewDiscoveryServer first")
	ERRSSTARTED = fmt.Errorf("[Discovery.server]already started")
)

//appuniquename = appname:ip:port
type discoveryserver struct {
	c           *stream.InstanceConfig
	lker        *sync.RWMutex
	allapps     map[string]*appnode //key appuniquename
	verifydata  []byte
	appnodepool *sync.Pool
	instance    *stream.Instance
	status      int32
}

//appuniquename = appname:ip:port
type appnode struct {
	appuniquename string
	peer          *stream.Peer
	starttime     uint64
	regmsg        *RegMsg
	regdata       []byte
	status        int //1 connected,2 preparing,3 registered
}

func (s *discoveryserver) getnode(peer *stream.Peer, appuniquename string, starttime uint64) *appnode {
	node, ok := s.appnodepool.Get().(*appnode)
	if !ok {
		return &appnode{
			appuniquename: appuniquename,
			peer:          peer,
			starttime:     starttime,
			status:        1,
		}
	}
	node.appuniquename = appuniquename
	node.peer = peer
	node.starttime = starttime
	node.status = 1
	return node
}

func (s *discoveryserver) putnode(n *appnode) {
	n.appuniquename = ""
	n.peer = nil
	n.starttime = 0
	n.regdata = nil
	n.status = 0
	s.appnodepool.Put(n)
}

var serverinstance *discoveryserver

func NewDiscoveryServer(c *stream.InstanceConfig, vdata []byte) {
	temp := &discoveryserver{
		lker:        &sync.RWMutex{},
		allapps:     make(map[string]*appnode),
		verifydata:  vdata,
		appnodepool: &sync.Pool{},
	}
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&serverinstance)), nil, unsafe.Pointer(temp)) {
		return
	}
	//tcp instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = serverinstance.onlinefunc
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = serverinstance.offlinefunc
	serverinstance.c = &dupc
	serverinstance.instance = stream.NewInstance(&dupc)
}
func StartDiscoveryServer(listenaddr string) error {
	if serverinstance == nil {
		return ERRSINIT
	}
	if old := atomic.SwapInt32(&serverinstance.status, 1); old == 1 {
		return ERRSSTARTED
	}
	serverinstance.instance.StartTcpServer(listenaddr)
	return nil
}

//appuniquename = appname:ip:port
func (s *discoveryserver) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	temp := common.Byte2str(peerVerifyData)
	index := strings.LastIndex(temp, "|")
	if index == -1 {
		return nil, false
	}
	targetname := temp[index+1:]
	vdata := temp[:index]
	if targetname != s.c.SelfName || vdata != hex.EncodeToString(s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

//appuniquename = appname:ip:port
func (s *discoveryserver) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	if _, ok := s.allapps[appuniquename]; ok {
		s.lker.Unlock()
		//this is impossible
		fmt.Printf("[Discovery.server.onlinefunc.impossible]duplicate connection from peer:%s\n", appuniquename)
		return
	}
	s.allapps[appuniquename] = s.getnode(p, appuniquename, starttime)
	s.lker.Unlock()
	fmt.Printf("[Discovery.server.onlinefunc]app:%s online\n", appuniquename)
}

//appuniquename = appname:ip:port
func (s *discoveryserver) userfunc(p *stream.Peer, appuniquename string, data []byte, starttime uint64) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case msgonline:
		_, regmsg, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]online message:%s broken\n", data)
			p.Close()
			return
		}
		reg := &RegMsg{}
		if e := json.Unmarshal(regmsg, reg); e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]app:%s register message:%s broken\n", appuniquename, regmsg)
			p.Close()
			return
		}
		if reg.GrpcPort == 0 && reg.HttpPort == 0 && reg.TcpPort == 0 && reg.WebSockPort == 0 {
			//register with empty data
			fmt.Printf("[Discovery.server.userfunc.impossible]app:%s register with empty message:%s\n", appuniquename, regmsg)
			p.Close()
			return
		}
		ip := appuniquename[strings.Index(appuniquename, ":")+1 : strings.LastIndex(appuniquename, ":")]
		if reg.GrpcPort != 0 && reg.GrpcIp == "" {
			reg.GrpcIp = ip
		}
		if reg.HttpPort != 0 && reg.HttpIp == "" {
			reg.HttpIp = ip
		}
		if reg.TcpPort != 0 && reg.TcpIp == "" {
			reg.TcpIp = ip
		}
		if reg.WebSockPort != 0 && reg.WebSockIp == "" {
			reg.WebSockIp = ip
		}
		regmsg, _ = json.Marshal(reg)
		s.lker.Lock()
		node, ok := s.allapps[appuniquename]
		if !ok {
			//this is impossible
			s.lker.Unlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", appuniquename)
			p.Close()
			return
		}
		node.regdata = regmsg
		node.status = 3
		onlinemsg := makeOnlineMsg(appuniquename, regmsg)
		//notice all other peers
		for _, app := range s.allapps {
			if app.status > 1 {
				app.peer.SendMessage(onlinemsg, app.starttime)
			}
		}
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.userfunc]app:%s registered with data:%s\n", appuniquename, regmsg)
	case msgpull:
		s.lker.RLock()
		node, ok := s.allapps[appuniquename]
		if !ok {
			//this is impossible
			s.lker.RUnlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", appuniquename)
			p.Close()
			return
		}
		if node.status < 2 {
			fmt.Printf("[Discovery.server.userfunc]peer:%s preparing\n", appuniquename)
			node.status = 2
		}
		all := make(map[string][]byte, int(float64(len(s.allapps))*1.3))
		for _, app := range s.allapps {
			if app.status == 3 {
				all[app.appuniquename] = app.regdata
			}
		}
		p.SendMessage(makePushMsg(all), starttime)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc.impossible]unknown message type from peer:%s\n", appuniquename)
		p.Close()
	}
}

//appuniquename = appname:ip:port
func (s *discoveryserver) offlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	node, ok := s.allapps[appuniquename]
	if !ok {
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.offlinefunc.impossible]peer:%s missing\n", appuniquename)
		return
	}
	delete(s.allapps, appuniquename)
	if node.status != 3 {
		s.putnode(node)
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.offlinefunc]app:%s offline\n", appuniquename)
		return
	}
	s.putnode(node)
	//notice all other peer
	offlinemsg := makeOfflineMsg(appuniquename)
	for _, app := range s.allapps {
		if app.status > 1 {
			app.peer.SendMessage(offlinemsg, app.starttime)
		}
	}
	s.lker.Unlock()
	fmt.Printf("[Discovery.server.offlinefunc]app:%s offline\n", appuniquename)
}
