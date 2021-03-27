package discovery

import (
	"bytes"
	"context"
	"encoding/json"
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
)

//in this function,call DiscoveryClient.UpdateDiscoveryServers() to update the discovery servers
type DiscoveryServerFinder func(chan struct{}, *DiscoveryClient)

type DiscoveryClient struct {
	verifydata  string
	tcpinstance *stream.Instance
	regdata     []byte
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
	starttime uint64
	allapps   map[string]map[string]*appnode //first key:appname,second key:appuniquename
	status    int                            //0-closing,1-start,2-verified,3-connected,4-registered
	regdata   []byte
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
				server.peer.Close()
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
				regdata:   c.regdata,
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
func RegisterSelf(regmsg *RegMsg) error {
	if instance == nil {
		return errors.New("[Discovery.client] not inited")
	}
	if regmsg == nil || (regmsg.RpcPort == 0 && regmsg.WebPort == 0) {
		return errors.New("[Discovery.client] register message empty")
	}
	if regmsg.RpcPort > 65535 || regmsg.RpcPort < 0 {
		return errors.New("[Discovery.client] regmsg's RpcPort out of range")
	}
	if regmsg.WebPort > 65535 || regmsg.WebPort < 0 {
		return errors.New("[Discovery.client] regmsg's WebPort out of range")
	}
	if regmsg.RpcPort == regmsg.WebPort {
		return errors.New("[Discovery.client] regmsg's RpcPort and WebPort conflict")
	}
	if regmsg.WebPort != 0 && regmsg.WebScheme != "http" && regmsg.WebScheme != "https" {
		return errors.New("[Discovery.client] regmsg missing WebScheme")
	}
	d, _ := json.Marshal(regmsg)
	if bytes.Contains(d, []byte{split}) {
		return errors.New("[Discovery.client] register message contains illegal character '|'")
	}
	instance.lker.Lock()
	defer instance.lker.Unlock()
	if instance.regdata != nil {
		if bytes.Equal(instance.regdata, d) {
			return nil
		}
		return errors.New("[Discovery.client] already registered")
	}
	instance.regdata = d
	for serveruniquename, server := range instance.servers {
		server.lker.Lock()
		server.regdata = d
		if server.status == 3 {
			server.status = 4
			log.Info("[Discovery.client.RegisterSelf] register to server:", serveruniquename, "with data:", common.Byte2str(instance.regdata))
			server.peer.SendMessage(makeOnlineMsg("useless", instance.regdata), server.starttime, true)
		}
		server.lker.Unlock()
	}
	return nil
}
func UnRegisterSelf() error {
	if instance == nil {
		return errors.New("[Discovery.client] not inited")
	}
	instance.lker.Lock()
	if instance.status != 0 {
		instance.status = 0
		for k, server := range instance.servers {
			server.lker.Lock()
			server.status = 0
			server.regdata = nil
			delete(instance.servers, k)
			server.lker.Unlock()
		}
	}
	instance.lker.Unlock()
	if instance.tcpinstance != nil {
		instance.tcpinstance.Stop()
	}
	return nil
}

func NoticeWebChanges(appname string) (chan struct{}, error) {
	if instance == nil {
		return nil, errors.New("[Discovery.client] not inited")
	}
	pullmsg := makePullMsg(appname)
	instance.lker.RLock()
	for _, server := range instance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(pullmsg, server.starttime, true)
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
	pullmsg := makePullMsg(appname)
	instance.lker.RLock()
	for _, server := range instance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(pullmsg, server.starttime, true)
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
					if app.regmsg.RpcPort == 0 {
						continue
					}
					addr = app.regmsg.RpcIp + ":" + strconv.Itoa(app.regmsg.RpcPort)
				case 2:
					if app.regmsg.WebPort == 0 || app.regmsg.WebScheme == "" {
						continue
					}
					addr = app.regmsg.WebScheme + "://" + app.regmsg.WebIp + ":" + strconv.Itoa(app.regmsg.WebPort)
				}
				if _, ok := result[addr]; !ok {
					result[addr] = make([]string, 0, 5)
				}
				result[addr] = append(result[addr], serveruniquename)
				resultaddition = app.regmsg.Addition
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
func (c *DiscoveryClient) onlinefunc(p *stream.Peer, serveruniquename string, starttime uint64) {
	c.lker.RLock()
	server, ok := c.servers[serveruniquename]
	if !ok {
		p.Close()
		//discovery server removed
		c.lker.RUnlock()
		return
	}
	server.lker.Lock()
	defer server.lker.Unlock()
	c.lker.RUnlock()
	if server.peer != nil || server.status != 2 || server.starttime != 0 {
		p.Close()
		return
	}
	log.Info("[Discovery.client.onlinefunc] server:", serveruniquename, "online")
	server.peer = p
	server.starttime = starttime
	server.status = 3
	p.SetData(unsafe.Pointer(server))
	if server.regdata != nil {
		server.status = 4
		log.Info("[Discovery.client.onlinefunc] register to server:", serveruniquename, "with data:", common.Byte2str(server.regdata))
		server.peer.SendMessage(makeOnlineMsg("useless", server.regdata), server.starttime, true)
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
		server.peer.SendMessage(makePullMsg(k), server.starttime, true)
	}
	return
}
func (c *DiscoveryClient) userfunc(p *stream.Peer, serveruniquename string, origindata []byte, starttime uint64) {
	if len(origindata) == 0 {
		return
	}
	data := make([]byte, len(origindata))
	copy(data, origindata)
	server := (*servernode)(p.GetData())
	if server == nil {
		return
	}
	server.lker.Lock()
	defer server.lker.Unlock()
	switch data[0] {
	case msgonline:
		appuniquename, regdata := getOnlineMsg(data)
		if appuniquename == "" || regdata == nil {
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "online app with online message:", common.Byte2str(data), "format error")
			p.Close()
			return
		}
		reg := &RegMsg{}
		if e := json.Unmarshal(regdata, reg); e != nil {
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "online app:", appuniquename, "with register message:", common.Byte2str(regdata), "format error:", e)
			p.Close()
			return
		}
		if reg.RpcPort == 0 && (reg.WebPort == 0 || reg.WebScheme == "") {
			log.Error("[Discovery.server.userfunc] server:", serveruniquename, "online app:", appuniquename, "with empty register message:", common.Byte2str(regdata))
			p.Close()
			return
		}
		appname := appuniquename[:strings.Index(appuniquename, ":")]
		if group, ok := server.allapps[appname]; ok {
			if _, ok := group[appuniquename]; ok {
				return
			}
		}
		node := &appnode{
			appuniquename: appuniquename,
			regdata:       regdata,
			regmsg:        reg,
		}
		if _, ok := server.allapps[appname]; !ok {
			server.allapps[appname] = make(map[string]*appnode, 5)
		}
		server.allapps[appname][appuniquename] = node
		c.nlker.RLock()
		c.notice(appname)
		c.nlker.RUnlock()
	case msgoffline:
		appuniquename := getOfflineMsg(data)
		if appuniquename == "" {
			//this is impossible
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "offline app with offline message:", common.Byte2str(data), "format error")
			p.Close()
			return
		}
		appname := appuniquename[:strings.Index(appuniquename, ":")]
		delete(server.allapps[appname], appuniquename)
		if len(server.allapps[appname]) == 0 {
			delete(server.allapps, appname)
		}
		c.nlker.RLock()
		c.notice(appname)
		c.nlker.RUnlock()
	default:
		log.Error("[Discovery.client.userfunc] unknown message type")
		p.Close()
	}
}
func (c *DiscoveryClient) offlinefunc(p *stream.Peer, serveruniquename string, starttime uint64) {
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
