package discovery

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
	"github.com/chenjie199234/Corelib/util/common"
	"github.com/golang/protobuf/proto"
)

//in this function,call DiscoveryClient.UpdateDiscoveryServers() to update the discovery servers
type DiscoveryServerFinder func(chan struct{}, *DiscoveryClient)

type DiscoveryClient struct {
	verifydata  string
	tcpinstance *stream.Instance
	reginfo     *RegInfo
	status      int32 //0-closing,1-working

	manually chan struct{}

	lker    *sync.RWMutex
	servers map[string]*servernode //key serveruniquename = servername:ip:port

	rpcnotices map[string]map[chan struct{}]struct{} //key appname(without ip and port)
	webnotices map[string]map[chan struct{}]struct{} //key appname(without ip and port)
	nlker      *sync.RWMutex
}

var instance *DiscoveryClient

//appuniquename = appname:ip:port
type servernode struct {
	lker      *sync.Mutex
	peer      *stream.Peer
	starttime int64
	allapps   map[string]map[string]*appnode //first key:appname,second key:appuniquename
	status    int                            //0-closing,1-start,2-verified,3-connected,4-registered
	reginfo   *RegInfo
}

//finder is to find the discovery servers
func NewDiscoveryClient(c *stream.InstanceConfig, selfgroup, selfname, verifydata string, finder DiscoveryServerFinder) error {
	if e := common.NameCheck(selfname, false, true, false, true); e != nil {
		return errors.New("[Discovery.client] selfname:" + selfname + " check error:" + e.Error())
	}
	if e := common.NameCheck(selfgroup, false, true, false, true); e != nil {
		return errors.New("[Discovery.client] selfgroup:" + selfgroup + " check error:" + e.Error())
	}
	selfappname := selfgroup + "." + selfname
	if e := common.NameCheck(selfappname, true, true, false, true); e != nil {
		return errors.New("[Discovery.client] selfappname:" + selfappname + " check error:" + e.Error())
	}
	if finder == nil {
		return errors.New("[Discovery.client] missing finder")
	}
	temp := &DiscoveryClient{
		status: 1,

		manually: make(chan struct{}, 1),

		lker:    &sync.RWMutex{},
		servers: make(map[string]*servernode, 5),

		webnotices: make(map[string]map[chan struct{}]struct{}, 5),
		rpcnotices: make(map[string]map[chan struct{}]struct{}, 5),
		nlker:      &sync.RWMutex{},
	}
	temp.verifydata = verifydata
	var dupc stream.InstanceConfig
	if c == nil {
		dupc = stream.InstanceConfig{}
	} else {
		dupc = *c //duplicate to remote the callback func race
	}
	//tcp instance
	dupc.Verifyfunc = temp.verifyfunc
	dupc.Onlinefunc = temp.onlinefunc
	dupc.Userdatafunc = temp.userfunc
	dupc.Offlinefunc = temp.offlinefunc
	temp.tcpinstance, _ = stream.NewInstance(&dupc, selfgroup, selfname)
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&instance)), nil, unsafe.Pointer(temp)) {
		return nil
	}
	log.Info("[Discovery.client] start with verifydata:", verifydata)
	go finder(instance.manually, instance)
	return nil
}

//server addr format: servername:ip:port
func (c *DiscoveryClient) UpdateDiscoveryServers(serveraddrs []string) {
	c.lker.Lock()
	defer c.lker.Unlock()
	if c.status == 0 {
		return
	}
	//delete offline server
	for serveruniquename, server := range c.servers {
		find := false
		for _, saddr := range serveraddrs {
			if saddr == serveruniquename {
				find = true
				break
			}
		}
		if !find {
			server.lker.Lock()
			delete(c.servers, serveruniquename)
			server.status = 0
			if server.peer != nil {
				server.peer.Close(server.starttime)
			}
			server.lker.Unlock()
		}
	}
	//online new server
	for _, saddr := range serveraddrs {
		//check saddr
		findex := strings.Index(saddr, ":")
		if findex == -1 || findex == 0 || findex == len(saddr)-1 {
			log.Error("[Discovery.client.UpdateDiscoveryServers] server addr:", saddr, "format error")
			continue
		}
		if _, e := net.ResolveTCPAddr("tcp", saddr[findex+1:]); e != nil {
			log.Error("[Discovery.client.updateserver] server addr:", saddr, "format error")
			continue
		}
		var server *servernode
		for serveruniquename, v := range c.servers {
			if serveruniquename == saddr {
				server = v
				break
			}
		}
		if server == nil {
			//this server not in the serverlist before
			server = &servernode{
				lker:      &sync.Mutex{},
				peer:      nil,
				starttime: 0,
				allapps:   make(map[string]map[string]*appnode, 5),
				status:    1,
				reginfo:   c.reginfo,
			}
			c.servers[saddr] = server
			go c.start(saddr[findex+1:], saddr[:findex])
		}
	}
}
func (c *DiscoveryClient) start(addr, servername string) {
	tempverifydata := c.verifydata + "|" + servername
	if r := c.tcpinstance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
		c.lker.RLock()
		server, ok := c.servers[servername+":"+addr]
		if !ok {
			//server removed
			c.lker.RUnlock()
			return
		}
		server.lker.Lock()
		c.lker.RUnlock()
		if server.status == 0 {
			server.lker.Unlock()
		} else {
			select {
			case c.manually <- struct{}{}:
			default:
			}
			server.status = 1
			server.lker.Unlock()
			time.Sleep(100 * time.Millisecond)
			go c.start(addr, servername)
		}
	}
}
func RegisterSelf(reginfo *RegInfo) error {
	if instance == nil {
		return errors.New("[Discovery.client] not inited")
	}
	if reginfo == nil || (reginfo.RpcPort == 0 && reginfo.WebPort == 0) {
		return errors.New("[Discovery.client] register message empty")
	}
	if reginfo.RpcPort > 65535 || reginfo.RpcPort < 0 {
		return errors.New("[Discovery.client] reginfo's RpcPort out of range")
	}
	if reginfo.WebPort > 65535 || reginfo.WebPort < 0 {
		return errors.New("[Discovery.client] reginfo's WebPort out of range")
	}
	if reginfo.RpcPort == reginfo.WebPort {
		return errors.New("[Discovery.client] reginfo's RpcPort and WebPort conflict")
	}
	if reginfo.WebPort != 0 && reginfo.WebScheme != "http" && reginfo.WebScheme != "https" {
		return errors.New("[Discovery.client] reginfo missing WebScheme")
	}
	instance.lker.Lock()
	defer instance.lker.Unlock()
	if instance.reginfo != nil {
		if instance.reginfo.WebPort == reginfo.WebPort &&
			instance.reginfo.WebScheme == reginfo.WebScheme &&
			instance.reginfo.RpcPort == reginfo.RpcPort {
			return nil
		}
		return errors.New("[Discovery.client] already registered")
	}
	instance.reginfo = reginfo
	for serveruniquename, server := range instance.servers {
		server.lker.Lock()
		server.reginfo = reginfo
		if server.status == 3 {
			server.status = 4
			log.Info("[Discovery.client.RegisterSelf] reg to server:", serveruniquename, "with rpc:", reginfo.RpcPort, "web:", reginfo.WebScheme, reginfo.WebPort)
			onlinemsg, _ := proto.Marshal(&Msg{
				MsgType: MsgType_Reg,
				MsgContent: &Msg_RegMsg{
					RegMsg: &RegMsg{
						RegInfo: reginfo,
					},
				},
			})
			server.peer.SendMessage(onlinemsg, server.starttime, true)
		}
		server.lker.Unlock()
	}
	return nil
}
func UnRegisterSelf() error {
	if instance == nil {
		return errors.New("[Discovery.client] not inited")
	}
	instance.lker.RLock()
	defer instance.lker.RUnlock()
	if len(instance.servers) > 0 {
		offlinemsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_UnReg,
		})
		for serveruniquename, server := range instance.servers {
			server.lker.Lock()
			server.status = 3
			server.reginfo = nil
			server.peer.SendMessage(offlinemsg, server.starttime, true)
			log.Info("[Discovery.client] unreg from server:", serveruniquename)
			server.lker.Unlock()
		}
	}
	return nil
}

func NoticeWebChanges(appname string) (chan struct{}, error) {
	if instance == nil {
		return nil, errors.New("[Discovery.client] not inited")
	}
	watchmsg, _ := proto.Marshal(&Msg{
		MsgType: MsgType_Watch,
		MsgContent: &Msg_WatchMsg{
			WatchMsg: &WatchMsg{
				AppName: appname,
			},
		},
	})
	instance.lker.RLock()
	for _, server := range instance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(watchmsg, server.starttime, true)
		}
		server.lker.Unlock()
	}
	instance.lker.RUnlock()
	instance.nlker.Lock()
	defer instance.nlker.Unlock()
	if _, ok := instance.webnotices[appname]; !ok {
		instance.webnotices[appname] = make(map[chan struct{}]struct{}, 10)
	}
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	instance.webnotices[appname][ch] = struct{}{}
	return ch, nil
}

func NoticeRpcChanges(appname string) (chan struct{}, error) {
	if instance == nil {
		return nil, errors.New("[Discovery.client] not inited")
	}
	watchmsg, _ := proto.Marshal(&Msg{
		MsgType: MsgType_Watch,
		MsgContent: &Msg_WatchMsg{
			WatchMsg: &WatchMsg{
				AppName: appname,
			},
		},
	})
	instance.lker.RLock()
	for _, server := range instance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(watchmsg, server.starttime, true)
		}
		server.lker.Unlock()
	}
	instance.lker.RUnlock()
	instance.nlker.Lock()
	defer instance.nlker.Unlock()
	if _, ok := instance.rpcnotices[appname]; !ok {
		instance.rpcnotices[appname] = make(map[chan struct{}]struct{}, 10)
	}
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	instance.rpcnotices[appname][ch] = struct{}{}
	return ch, nil
}

//first return value
//key:app addr
//value:discovery server addrs
//second return value
//addition info
func GetRpcInfos(appname string) (map[string][]string, []byte) {
	if instance == nil {
		return nil, nil
	}
	return getinfos(appname, 1)
}

//first return value
//key:app addr
//value:discovery server addrs
//second return value
//addition info
func GetWebInfos(appname string) (map[string][]string, []byte) {
	if instance == nil {
		return nil, nil
	}
	return getinfos(appname, 2)
}

//first key:app addr
//second key:discovery addr
//value:addition data
func getinfos(appname string, t int) (map[string][]string, []byte) {
	result := make(map[string][]string, 5)
	var resultaddition []byte
	instance.lker.RLock()
	defer instance.lker.RUnlock()
	for serveruniquename, server := range instance.servers {
		server.lker.Lock()
		if appgroup, ok := server.allapps[appname]; ok {
			for _, app := range appgroup {
				var addr string
				switch t {
				case 1:
					if app.reginfo.RpcPort == 0 {
						continue
					}
					addr = app.reginfo.RpcIp + ":" + strconv.FormatInt(app.reginfo.RpcPort, 10)
				case 2:
					if app.reginfo.WebPort == 0 || app.reginfo.WebScheme == "" {
						continue
					}
					addr = app.reginfo.WebScheme + "://" + app.reginfo.WebIp + ":" + strconv.FormatInt(app.reginfo.WebPort, 10)
				}
				if _, ok := result[addr]; !ok {
					result[addr] = make([]string, 0, 5)
				}
				result[addr] = append(result[addr], serveruniquename)
				resultaddition = app.reginfo.Addition
			}
		}
		server.lker.Unlock()
	}
	return result, resultaddition
}

func (c *DiscoveryClient) verifyfunc(ctx context.Context, serveruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, common.Str2byte(c.verifydata)) {
		return nil, false
	}
	c.lker.RLock()
	server, ok := c.servers[serveruniquename]
	if !ok {
		//discovery server removed
		c.lker.RUnlock()
		return nil, false
	}
	server.lker.Lock()
	defer server.lker.Unlock()
	c.lker.RUnlock()
	if server.peer != nil || server.status != 1 || server.starttime != 0 {
		return nil, false
	}
	server.status = 2
	return nil, true
}
func (c *DiscoveryClient) onlinefunc(p *stream.Peer, serveruniquename string, starttime int64) {
	c.lker.RLock()
	server, ok := c.servers[serveruniquename]
	if !ok {
		p.Close(starttime)
		//discovery server removed
		c.lker.RUnlock()
		return
	}
	server.lker.Lock()
	defer server.lker.Unlock()
	c.lker.RUnlock()
	if server.peer != nil || server.status != 2 || server.starttime != 0 {
		p.Close(starttime)
		return
	}
	log.Info("[Discovery.client.onlinefunc] server:", serveruniquename, "online")
	server.peer = p
	server.starttime = starttime
	server.status = 3
	p.SetData(unsafe.Pointer(server))
	if server.reginfo != nil {
		server.status = 4
		log.Info("[Discovery.client.onlinefunc] reg to server:", serveruniquename, "with rpc:", server.reginfo.RpcPort, "web:", server.reginfo.WebScheme, server.reginfo.WebPort)
		onlinemsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_Reg,
			MsgContent: &Msg_RegMsg{
				RegMsg: &RegMsg{
					RegInfo: server.reginfo,
				},
			},
		})
		server.peer.SendMessage(onlinemsg, server.starttime, true)
	}
	c.nlker.RLock()
	result := make(map[string]struct{}, 5)
	for k := range c.rpcnotices {
		result[k] = struct{}{}
	}
	for k := range c.webnotices {
		result[k] = struct{}{}
	}
	c.nlker.RUnlock()
	for k := range result {
		watchmsg, _ := proto.Marshal(&Msg{
			MsgType: MsgType_Watch,
			MsgContent: &Msg_WatchMsg{
				WatchMsg: &WatchMsg{
					AppName: k,
				},
			},
		})
		server.peer.SendMessage(watchmsg, server.starttime, true)
	}
	return
}
func (c *DiscoveryClient) userfunc(p *stream.Peer, serveruniquename string, origindata []byte, starttime int64) {
	if len(origindata) == 0 {
		return
	}
	data := make([]byte, len(origindata))
	copy(data, origindata)
	msg := &Msg{}
	if e := proto.Unmarshal(data, msg); e != nil {
		log.Error("[Discovery.client.userfunc] message from:", serveruniquename, "format error:", e)
		p.Close(starttime)
		return
	}
	server := (*servernode)(p.GetData())
	if server == nil {
		return
	}
	server.lker.Lock()
	defer server.lker.Unlock()
	switch msg.MsgType {
	case MsgType_Reg:
		reg := msg.GetRegMsg()
		if reg == nil || reg.AppUniqueName == "" || reg.RegInfo == nil || ((reg.RegInfo.WebPort == 0 || reg.RegInfo.WebScheme == "") && reg.RegInfo.RpcPort == 0) {
			log.Error("[Discovery.client.userfunc] empty reg msg from:", serveruniquename)
			p.Close(starttime)
			return
		}
		appname := reg.AppUniqueName[:strings.Index(reg.AppUniqueName, ":")]
		if group, ok := server.allapps[appname]; ok {
			if _, ok := group[reg.AppUniqueName]; ok {
				return
			}
		}
		node := &appnode{
			appuniquename: reg.AppUniqueName,
			reginfo:       reg.RegInfo,
		}
		if _, ok := server.allapps[appname]; !ok {
			server.allapps[appname] = make(map[string]*appnode, 5)
		}
		server.allapps[appname][reg.AppUniqueName] = node
		c.nlker.RLock()
		c.notice(appname)
		c.nlker.RUnlock()
	case MsgType_UnReg:
		unreg := msg.GetUnregMsg()
		if unreg.AppUniqueName == "" {
			log.Error("[Discovery.client.userfunc] empty unreg msg from:", serveruniquename)
			p.Close(starttime)
			return
		}
		appname := unreg.AppUniqueName[:strings.Index(unreg.AppUniqueName, ":")]
		delete(server.allapps[appname], unreg.AppUniqueName)
		if len(server.allapps[appname]) == 0 {
			delete(server.allapps, appname)
		}
		c.nlker.RLock()
		c.notice(appname)
		c.nlker.RUnlock()
	case MsgType_Push:
		push := msg.GetPushMsg()
		if push == nil || push.AppName == "" {
			log.Error("[Discovery.client.userfunc] empty push msg from:", serveruniquename)
			p.Close(starttime)
			return
		}
		if _, ok := server.allapps[push.AppName]; !ok {
			server.allapps[push.AppName] = make(map[string]*appnode, 5)
		}
		//offline
		for appuniquename := range server.allapps[push.AppName] {
			if _, ok := push.Apps[appuniquename]; !ok {
				delete(server.allapps, appuniquename)
			}
		}
		//online
		for appuniquename, reginfo := range push.Apps {
			server.allapps[push.AppName][appuniquename] = &appnode{
				appuniquename: appuniquename,
				reginfo:       reginfo,
			}
		}
		c.nlker.RLock()
		c.notice(push.AppName)
		c.nlker.RUnlock()
	default:
		log.Error("[Discovery.client.userfunc] unknown message type")
		p.Close(starttime)
	}
}
func (c *DiscoveryClient) offlinefunc(p *stream.Peer, serveruniquename string) {
	server := (*servernode)(p.GetData())
	if server == nil {
		return
	}
	log.Info("[Discovery.client.offlinefunc] server:", serveruniquename, "offline")
	server.lker.Lock()
	server.peer = nil
	server.starttime = 0
	//notice
	c.nlker.Lock()
	for appname := range server.allapps {
		c.notice(appname)
	}
	c.nlker.Unlock()
	server.allapps = make(map[string]map[string]*appnode, 5)
	if server.status == 0 {
		server.lker.Unlock()
	} else {
		select {
		case c.manually <- struct{}{}:
		default:
		}
		server.status = 1
		server.lker.Unlock()
		time.Sleep(100 * time.Millisecond)
		index := strings.Index(serveruniquename, ":")
		go c.start(serveruniquename[index+1:], serveruniquename[:index])
	}
}
func (c *DiscoveryClient) notice(appname string) {
	if notices, ok := c.rpcnotices[appname]; ok {
		for n := range notices {
			select {
			case n <- struct{}{}:
			default:
			}
		}
	}
	if notices, ok := c.webnotices[appname]; ok {
		for n := range notices {
			select {
			case n <- struct{}{}:
			default:
			}
		}
	}
}
