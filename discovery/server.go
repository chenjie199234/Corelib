package discovery

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
)

//appuniquename = appname:ip:port
type discoveryserver struct {
	selfname    string
	lker        *sync.RWMutex
	allapps     map[string]*appnode //key appuniquename
	verifydata  []byte
	appnodepool *sync.Pool
	instance    *stream.Instance
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

//var serverinstance

func NewDiscoveryServer(c *stream.InstanceConfig, vdata []byte) *discoveryserver {
	instance := &discoveryserver{
		selfname:    c.SelfName,
		lker:        &sync.RWMutex{},
		allapps:     make(map[string]*appnode),
		verifydata:  vdata,
		appnodepool: &sync.Pool{},
	}
	//tcp instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = instance.verifyfunc
	dupc.Onlinefunc = instance.onlinefunc
	dupc.Userdatafunc = instance.userfunc
	dupc.Offlinefunc = instance.offlinefunc
	instance.instance = stream.NewInstance(&dupc)
	return instance
}
func (s *discoveryserver) StartDiscoveryServer(listenaddr string) {
	s.instance.StartTcpServer(listenaddr)
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
	if targetname != s.selfname || vdata != common.Byte2str(s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

//appuniquename = appname:ip:port
func (s *discoveryserver) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	if _, ok := s.allapps[appuniquename]; ok {
		p.Close()
		s.lker.Unlock()
		return
	}
	s.allapps[appuniquename] = s.getnode(p, appuniquename, starttime)
	s.lker.Unlock()
	log.Info("[Discovery.server.onlinefunc]app:", appuniquename, "online")
	return
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
			log.Error("[Discovery.server.userfunc]app:", appuniquename, "online message:", common.Byte2str(data), "format unknown")
			p.Close()
			return
		}
		reg := &RegMsg{}
		if e := json.Unmarshal(regmsg, reg); e != nil {
			//this is impossible
			log.Error("[Discovery.server.userfunc]app:", appuniquename, "register message:", common.Byte2str(regmsg), "format error:", e)
			p.Close()
			return
		}
		if (reg.WebPort == 0 || reg.WebScheme == "") && reg.RpcPort == 0 {
			//register with empty data
			log.Error("[Discovery.server.userfunc]app:", appuniquename, "register with empty message:", common.Byte2str(regmsg))
			p.Close()
			return
		}
		ip := appuniquename[strings.Index(appuniquename, ":")+1 : strings.LastIndex(appuniquename, ":")]
		if reg.WebPort != 0 && reg.WebScheme != "" && reg.WebIp == "" {
			reg.WebIp = ip
		}
		if reg.RpcPort != 0 && reg.RpcIp == "" {
			reg.RpcIp = ip
		}
		regmsg, _ = json.Marshal(reg)
		s.lker.Lock()
		node, ok := s.allapps[appuniquename]
		if !ok {
			//this is impossible
			s.lker.Unlock()
			log.Error("[Discovery.server.userfunc]app:", appuniquename, "missing")
			p.Close()
			return
		}
		node.regdata = regmsg
		node.status = 3
		onlinemsg := makeOnlineMsg(appuniquename, regmsg)
		//notice all other peers
		for _, app := range s.allapps {
			if app.status > 1 {
				app.peer.SendMessage(onlinemsg, app.starttime, true)
			}
		}
		s.lker.Unlock()
		log.Info("[Discovery.server.userfunc]app:", appuniquename, "registered with data:", common.Byte2str(regmsg))
	case msgpull:
		s.lker.RLock()
		node, ok := s.allapps[appuniquename]
		if !ok {
			//this is impossible
			s.lker.RUnlock()
			log.Error("[Discovery.server.userfunc]app:", appuniquename, "missing")
			p.Close()
			return
		}
		if node.status < 2 {
			log.Info("[Discovery.server.userfunc]app:", appuniquename, "preparing")
			node.status = 2
		}
		all := make(map[string][]byte, int(float64(len(s.allapps))*1.3))
		for _, app := range s.allapps {
			if app.status == 3 {
				all[app.appuniquename] = app.regdata
			}
		}
		p.SendMessage(makePushMsg(all), starttime, true)
		s.lker.RUnlock()
	default:
		log.Error("[Discovery.server.userfunc]unknown message type from app:", appuniquename)
		p.Close()
	}
}

//appuniquename = appname:ip:port
func (s *discoveryserver) offlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	node, ok := s.allapps[appuniquename]
	if !ok {
		s.lker.Unlock()
		log.Error("[Discovery.server.offlinefunc]app:", appuniquename, "missing")
		return
	}
	delete(s.allapps, appuniquename)
	if node.status != 3 {
		s.putnode(node)
		s.lker.Unlock()
		log.Info("[Discovery.server.offlinefunc]app:", appuniquename, "offline")
		return
	}
	s.putnode(node)
	//notice all other peer
	offlinemsg := makeOfflineMsg(appuniquename)
	for _, app := range s.allapps {
		if app.status > 1 {
			app.peer.SendMessage(offlinemsg, app.starttime, true)
		}
	}
	s.lker.Unlock()
	log.Info("[Discovery.server.offlinefunc]app:", appuniquename, "offline")
}
