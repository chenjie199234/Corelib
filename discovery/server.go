package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"google.golang.org/protobuf/proto"
)

const (
	s_CLOSED = iota
	s_CONNECTED
	s_REGISTERED
)

type DiscoveryServer struct {
	lker        *sync.RWMutex
	groups      map[string]*appgroup //key appname
	verifydatas []string
	instance    *stream.Instance
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
	reginfo       *RegInfo
	watched       map[string]struct{} //key appname
	bewatched     map[string]*appnode //key appuniquename
}

//old: true means oldverifydata is useful,false means oldverifydata is useless
func NewDiscoveryServer(c *stream.InstanceConfig, group, name string, verifydatas []string) (*DiscoveryServer, error) {
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
		lker:        &sync.RWMutex{},
		groups:      make(map[string]*appgroup, 5),
		verifydatas: verifydatas,
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
	log.Info("[Discovery.server] start with verifydatas:", s.verifydatas)
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
func (s *DiscoveryServer) GetAppInfo(appname string) map[string]*Info {
	result := make(map[string]*Info)
	s.lker.RLock()
	defer s.lker.RUnlock()
	if group, ok := s.groups[appname]; ok {
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
		fmt.Println(1)
		return nil, false
	}
	targetname := temp[index+1:]
	vdata := temp[:index]
	if targetname != s.instance.GetSelfName() {
		fmt.Println(2)
		return nil, false
	}
	if len(s.verifydatas) == 0 {
		dup := make([]byte, len(vdata))
		copy(dup, vdata)
		return dup, true
	}
	for _, verifydata := range s.verifydatas {
		if verifydata == vdata {
			return common.Str2byte(verifydata), true
		}
	}
	fmt.Println(3)
	return nil, false
}

//appuniquename = appname:ip:port
func (s *DiscoveryServer) onlinefunc(p *stream.Peer, appuniquename string, starttime uint64) {
	s.lker.Lock()
	defer s.lker.Unlock()
	appname := appuniquename[:strings.Index(appuniquename, ":")]
	if g, ok := s.groups[appname]; ok {
		if _, ok := g.apps[appuniquename]; ok {
			p.Close()
			return
		}
	} else {
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
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error("[Discovery.server.userfunc] message from:", appuniquename, "format error:", e)
		p.Close()
		return
	}
	switch msg.MsgType {
	case MsgType_Reg:
		reg := msg.GetRegMsg()
		if reg == nil || reg.RegInfo == nil || ((reg.RegInfo.WebPort == 0 || reg.RegInfo.WebScheme == "") && reg.RegInfo.RpcPort == 0) {
			//register with empty data
			log.Error("[Discovery.server.userfunc] empty reginfo from:", appuniquename)
			p.Close()
			return
		}
		findex := strings.Index(appuniquename, ":")
		lindex := strings.LastIndex(appuniquename, ":")
		ip := appuniquename[findex+1 : lindex]
		reg.RegInfo.WebIp = ""
		if reg.RegInfo.WebPort != 0 && reg.RegInfo.WebScheme != "" {
			reg.RegInfo.WebIp = ip
		}
		reg.RegInfo.RpcIp = ""
		if reg.RegInfo.RpcPort != 0 {
			reg.RegInfo.RpcIp = ip
		}
		node := (*appnode)(p.GetData())
		node.lker.Lock()
		defer node.lker.Unlock()
		node.reginfo = reg.RegInfo
		node.status = s_REGISTERED
		onlinemsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_Reg,
			MsgContent: &Msg_RegMsg{
				RegMsg: &RegMsg{
					AppUniqueName: appuniquename,
					RegInfo:       reg.RegInfo,
				},
			},
		})
		for _, v := range node.bewatched {
			v.lker.RLock()
			if v.status != s_CLOSED {
				v.peer.SendMessage(onlinemsg, v.starttime, true)
			}
			v.lker.RUnlock()
		}
		if reg.RegInfo.WebIp != "" && reg.RegInfo.RpcIp != "" {
			log.Info("[Discovery.server.userfunc] app:", appuniquename, "reg with rpc:", ip, reg.RegInfo.RpcPort, "web:", reg.RegInfo.WebScheme, ip, reg.RegInfo.WebPort)
		} else if reg.RegInfo.WebIp != "" {
			log.Info("[Discovery.server.userfunc] app:", appuniquename, "reg with web:", reg.RegInfo.WebScheme, ip, reg.RegInfo.WebPort)
		} else {
			log.Info("[Discovery.server.userfunc] app:", appuniquename, "reg with rpc:", ip, reg.RegInfo.RpcPort)
		}
	case MsgType_Watch:
		watch := msg.GetWatchMsg()
		if watch.AppName == "" {
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "watch empty")
			p.Close()
			return
		}
		if watch.AppName == appuniquename[:strings.Index(appuniquename, ":")] {
			log.Error("[Discovery.server.userfunc] app:", appuniquename, "watch self")
			p.Close()
			return
		}
		node := (*appnode)(p.GetData())
		s.lker.Lock()
		if _, ok := s.groups[watch.AppName]; !ok {
			s.groups[watch.AppName] = &appgroup{
				apps:     make(map[string]*appnode, 5),
				watchers: make(map[string]*appnode, 5),
			}
		}
		s.groups[watch.AppName].watchers[appuniquename] = node
		apps := make(map[string]*RegInfo, len(s.groups[watch.AppName].apps))
		for _, app := range s.groups[watch.AppName].apps {
			app.lker.Lock()
			app.bewatched[appuniquename] = node
			if app.status == s_REGISTERED {
				apps[app.appuniquename] = app.reginfo
			}
			app.lker.Unlock()
		}
		node.lker.Lock()
		s.lker.Unlock()
		node.watched[watch.AppName] = struct{}{}
		pushmsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_Push,
			MsgContent: &Msg_PushMsg{
				PushMsg: &PushMsg{
					AppName: watch.AppName,
					Apps:    apps,
				},
			},
		})
		node.peer.SendMessage(pushmsg, node.starttime, true)
		node.lker.Unlock()
	case MsgType_UnReg:
		node := (*appnode)(p.GetData())
		node.lker.Lock()
		defer node.lker.Unlock()
		if node.status == s_REGISTERED {
			node.status = s_CONNECTED
			if len(node.bewatched) > 0 {
				offlinemsg, _ := proto.Marshal(&Msg{
					MsgType: MsgType_UnReg,
					MsgContent: &Msg_UnregMsg{
						UnregMsg: &UnregMsg{
							AppUniqueName: appuniquename,
						},
					},
				})
				for _, watchedapp := range node.bewatched {
					watchedapp.lker.RLock()
					if watchedapp.status != s_CLOSED {
						watchedapp.peer.SendMessage(offlinemsg, watchedapp.starttime, true)
					}
					watchedapp.lker.RUnlock()
				}
			}
		}
		log.Info("[Discovery.server.userfunc] app:", appuniquename, "unreg")
	default:
		log.Error("[Discovery.server.userfunc] unknown message type:", msg.MsgType, "from app:", appuniquename)
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
		offlinemsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_UnReg,
			MsgContent: &Msg_UnregMsg{
				UnregMsg: &UnregMsg{
					AppUniqueName: appuniquename,
				},
			},
		})
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
