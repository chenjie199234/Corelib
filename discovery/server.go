package discovery

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
)

const (
	s_CLOSED = iota
	s_CONNECTED
	s_REGISTERED
)

type DiscoveryServer struct {
	lker       *sync.RWMutex
	groups     map[string]*appgroup //key appname
	verifydata []byte
	instance   *stream.Instance
}

//appuniquename = appname:ip:port
type appgroup struct {
	apps     map[string]*appnode //key appuniquename
	watchers map[string]*appnode //key appuniquename
}

//appuniquename = appname:ip:port
type appnode struct {
	lker          *sync.RWMutex
	appuniquename string
	peer          *stream.Peer
	starttime     uint64
	status        int
	regmsg        *RegMsg
	regdata       []byte
	watched       map[string]struct{} //key appname
	bewatched     map[string]*appnode //key appuniquename
}

func NewDiscoveryServer(c *stream.InstanceConfig, group, name string, vdata []byte) (*DiscoveryServer, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group+"."+name, true, true, false, true); e != nil {
		return nil, e
	}
	instance := &DiscoveryServer{
		lker:       &sync.RWMutex{},
		groups:     make(map[string]*appgroup, 5),
		verifydata: vdata,
	}
	var dupc stream.InstanceConfig
	if c == nil {
		dupc = stream.InstanceConfig{}
	} else {
		dupc = *c //duplicate to remote the callback func race
	}
	//tcp instance
	dupc.Verifyfunc = instance.verifyfunc
	dupc.Onlinefunc = instance.onlinefunc
	dupc.Userdatafunc = instance.userfunc
	dupc.Offlinefunc = instance.offlinefunc
	instance.instance, _ = stream.NewInstance(&dupc, group, name)
	return instance, nil
}

func (s *DiscoveryServer) StartDiscoveryServer(listenaddr string) error {
	return s.instance.StartTcpServer(listenaddr)
}
func (s *DiscoveryServer) StopDiscoveryServer() {
	s.instance.Stop()
}

//one app's info
type Info struct {
	Apps     map[string]int //key appuniquename,value registered status
	Watchers []string       //value appuniquename
}

func (s *DiscoveryServer) GetAppInfos() map[string]*Info {
	result := make(map[string]*Info)
	s.lker.RLock()
	defer s.lker.RUnlock()
	for appname, group := range s.groups {
		temp := &Info{
			Apps:     make(map[string]int, len(group.apps)),
			Watchers: make([]string, 0, len(group.watchers)),
		}
		for k, app := range group.apps {
			temp.Apps[k] = app.status
		}
		for k := range group.watchers {
			temp.Watchers = append(temp.Watchers, k)
		}
		result[appname] = temp
	}
	return result
}

//appuniquename = appname:ip:port
func (s *DiscoveryServer) verifyfunc(ctx context.Context, appuniquename string, peerVerifyData []byte) ([]byte, bool) {
	temp := common.Byte2str(peerVerifyData)
	index := strings.LastIndex(temp, "|")
	if index == -1 {
		return nil, false
	}
	targetname := temp[index+1:]
	vdata := temp[:index]
	if targetname != s.instance.GetSelfName() || vdata != common.Byte2str(s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

//appuniquename = appname:ip:port
func (s *DiscoveryServer) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	appname := appuniquename[:strings.Index(appuniquename, ":")]
	if g, ok := s.groups[appname]; ok {
		if _, ok := g.apps[appuniquename]; ok {
			p.Close()
			s.lker.Unlock()
			return
		}
	}
	if _, ok := s.groups[appname]; !ok {
		s.groups[appname] = &appgroup{
			apps:     make(map[string]*appnode, 5),
			watchers: make(map[string]*appnode, 5),
		}
	}
	node := &appnode{
		lker:          new(sync.RWMutex),
		appuniquename: appuniquename,
		peer:          p,
		starttime:     starttime,
		status:        s_CONNECTED,
		watched:       make(map[string]struct{}, 5),
		bewatched:     make(map[string]*appnode, 5),
	}
	//copy bewarched
	for k, v := range s.groups[appname].watchers {
		node.bewatched[k] = v
	}
	p.SetData(unsafe.Pointer(node))
	s.groups[appname].apps[appuniquename] = node
	s.lker.Unlock()
	log.Info("[Discovery.server.onlinefunc] app:", appuniquename, "online")
	return
}

//appuniquename = appname:ip:port
func (s *DiscoveryServer) userfunc(p *stream.Peer, appuniquename string, origindata []byte, starttime uint64) {
	if len(origindata) == 0 {
		return
	}
	data := make([]byte, len(origindata))
	copy(data, origindata)
	switch data[0] {
	case msgonline:
		_, regdata := getOnlineMsg(data)
		if len(regdata) == 0 {
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "online message:", common.Byte2str(regdata), "format error")
			p.Close()
			return
		}
		reg := &RegMsg{}
		if e := json.Unmarshal(regdata, reg); e != nil {
			//this is impossible
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "register message:", common.Byte2str(regdata), "format error:", e)
			p.Close()
			return
		}
		if (reg.WebPort == 0 || reg.WebScheme == "") && reg.RpcPort == 0 {
			//register with empty data
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "with empty register message:", common.Byte2str(regdata))
			p.Close()
			return
		}
		findex := strings.Index(appuniquename, ":")
		lindex := strings.LastIndex(appuniquename, ":")
		ip := appuniquename[findex+1 : lindex]
		if reg.WebPort != 0 && reg.WebScheme != "" && reg.WebIp == "" {
			reg.WebIp = ip
		}
		if reg.RpcPort != 0 && reg.RpcIp == "" {
			reg.RpcIp = ip
		}
		regdata, _ = json.Marshal(reg)
		node := (*appnode)(p.GetData())
		node.lker.Lock()
		defer node.lker.Unlock()
		node.regdata = regdata
		node.status = s_REGISTERED
		onlinemsg := makeOnlineMsg(appuniquename, regdata)
		for _, v := range node.bewatched {
			v.lker.RLock()
			if v.status != s_CLOSED {
				v.peer.SendMessage(onlinemsg, v.starttime, true)
			}
			v.lker.RUnlock()
		}
		log.Info("[Discovery.server.userfunc] app:", appuniquename, "registered with data:", common.Byte2str(regdata))
	case msgpull:
		temp := make([]byte, len(data))
		copy(temp, data)
		appname := getPullMsg(temp)
		if appname == "" {
			return
		}
		if appname == appuniquename[:strings.Index(appuniquename, ":")] {
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "self watching")
			return
		}
		node := (*appnode)(p.GetData())
		s.lker.Lock()
		if _, ok := s.groups[appname]; !ok {
			s.groups[appname] = &appgroup{
				apps:     make(map[string]*appnode, 5),
				watchers: make(map[string]*appnode, 5),
			}
		}
		s.groups[appname].watchers[appuniquename] = node
		node.lker.Lock()
		result := make(map[string][]byte, len(s.groups[appname].apps))
		for _, v := range s.groups[appname].apps {
			v.lker.Lock()
			v.bewatched[appuniquename] = node
			if v.status == s_REGISTERED {
				result[v.appuniquename] = v.regdata
			}
			v.lker.Unlock()
		}
		s.lker.Unlock()
		node.watched[appname] = struct{}{}
		for k, v := range result {
			node.peer.SendMessage(makeOnlineMsg(k, v), node.starttime, true)
		}
		node.lker.Unlock()
	default:
		log.Error("[Discovery.server.userfunc] unknown message type from app:", appuniquename)
		p.Close()
	}
}

//appuniquename = appname:ip:port
func (s *DiscoveryServer) offlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	appname := appuniquename[:strings.Index(appuniquename, ":")]
	group, ok := s.groups[appname]
	if !ok {
		s.lker.Unlock()
		log.Error("[Discovery.server.offlinefunc] app:", appuniquename, "missing")
		return
	}
	node, ok := group.apps[appuniquename]
	if !ok {
		s.lker.Unlock()
		log.Error("[Discovery.server.offlinefunc] app:", appuniquename, "missing")
		return
	}
	delete(s.groups[appname].apps, appuniquename)
	if len(s.groups[appname].apps) == 0 && len(s.groups[appname].watchers) == 0 {
		delete(s.groups, appname)
	}
	for v := range node.watched {
		group, ok := s.groups[v]
		if !ok {
			continue
		}
		delete(group.watchers, appuniquename)
		for _, app := range group.apps {
			app.lker.Lock()
			delete(app.bewatched, appuniquename)
			app.lker.Unlock()
		}
		if len(group.apps) == 0 && len(group.watchers) == 0 {
			delete(s.groups, v)
		}
	}
	node.lker.Lock()
	s.lker.Unlock()
	if node.status == s_REGISTERED && len(node.bewatched) > 0 {
		offlinemsg := makeOfflineMsg(appuniquename)
		for _, v := range node.bewatched {
			v.lker.RLock()
			if v.status != s_CLOSED {
				v.peer.SendMessage(offlinemsg, v.starttime, true)
			}
			v.lker.RUnlock()
		}
	}
	node.status = s_CLOSED
	node.lker.Unlock()
	log.Info("[Discovery.server.offlinefunc] app:", appuniquename, "offline")
}
