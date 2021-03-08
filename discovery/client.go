package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var (
	ERRINIT        = errors.New("[Discovery.client] not init,call NewDiscoveryClient first")
	ERRREG         = errors.New("[Discovery.client] already registered self")
	ERRREGMSG_PORT = errors.New("[Discovery.client] reg message port conflict")
	ERRREGMSG_CHAR = errors.New("[Discovery.client] reg message contains illegal character '|'")
	ERRREGMSG_NIL  = errors.New("[Discovery.client] reg message empty")
)

//in this function,call UpdateDiscoveryServers() to update the discovery servers
type DiscoveryServerFinder func(manually chan struct{})

//serveruniquename = servername:ip:port
type DiscoveryClient struct {
	selfname   string
	verifydata []byte
	instance   *stream.Instance
	regdata    []byte
	status     int //0-closing,1-working

	//finder
	finder   DiscoveryServerFinder
	manually chan struct{}
	//other
	lker    *sync.RWMutex
	servers map[string]*servernode //key serveruniquename
	//key appname
	rpcnotices map[string]map[chan struct{}]struct{}
	webnotices map[string]map[chan struct{}]struct{}
	nlker      *sync.RWMutex
}

//appuniquename = appname:ip:port
type servernode struct {
	lker      *sync.Mutex
	peer      *stream.Peer
	starttime uint64
	allapps   map[string]map[string]*appnode //first key:appname,second key:appuniquename
	status    int                            //0-closing,1-start,2-verified,3-connected,4-registered
	regdata   []byte
}

var clientinstance *DiscoveryClient

//this just start the client and sync the peers in the net
//this will not register self into the net
//please call the RegisterSelf() func to register self into the net
func NewDiscoveryClient(c *stream.InstanceConfig, vdata []byte, finder DiscoveryServerFinder) error {
	if finder == nil {
		return errors.New("[Discovery.client] finder nil")
	}
	temp := &DiscoveryClient{
		selfname:   c.SelfName,
		verifydata: vdata,
		status:     1,
		finder:     finder,
		manually:   make(chan struct{}, 1),
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*servernode, 5),
		webnotices: make(map[string]map[chan struct{}]struct{}, 5),
		rpcnotices: make(map[string]map[chan struct{}]struct{}, 5),
		nlker:      &sync.RWMutex{},
	}
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&clientinstance)), nil, unsafe.Pointer(temp)) {
		return nil
	}
	//tcp instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = clientinstance.verifyfunc
	dupc.Onlinefunc = clientinstance.onlinefunc
	dupc.Userdatafunc = clientinstance.userfunc
	dupc.Offlinefunc = clientinstance.offlinefunc
	var e error
	clientinstance.instance, e = stream.NewInstance(&dupc)
	if e != nil {
		return errors.New("[discovery.client.NewDiscoveryClient]new tcp instance error:" + e.Error())
	}
	go finder(clientinstance.manually)
	return nil
}

//server addr format: servername:ip:port
func UpdateDiscoveryServers(serveraddrs []string) {
	clientinstance.lker.Lock()
	defer clientinstance.lker.Unlock()
	if clientinstance.status == 0 {
		//clientinstance.lker.Unlock()
		return
	}
	//delete offline server
	for serveruniquename, server := range clientinstance.servers {
		find := false
		for _, saddr := range serveraddrs {
			if saddr == serveruniquename {
				find = true
				break
			}
		}
		if !find {
			server.lker.Lock()
			delete(clientinstance.servers, serveruniquename)
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
		for serveruniquename, v := range clientinstance.servers {
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
				regdata:   clientinstance.regdata,
			}
			clientinstance.servers[saddr] = server
			go clientinstance.start(saddr[findex+1:], saddr[:findex])
		}
	}
}
func (c *DiscoveryClient) start(addr, servername string) {
	tempverifydata := common.Byte2str(clientinstance.verifydata) + "|" + servername
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
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
	if clientinstance == nil {
		return ERRINIT
	}
	if regmsg == nil {
		return ERRREGMSG_NIL
	}
	temp := make(map[int]struct{})
	count := 0
	if regmsg.WebPort != 0 && regmsg.WebScheme != "" {
		temp[regmsg.WebPort] = struct{}{}
		count++
	}
	if regmsg.RpcPort != 0 {
		temp[regmsg.RpcPort] = struct{}{}
		count++
	}
	if count == 0 {
		return ERRREGMSG_NIL
	}
	if len(temp) != count {
		return ERRREGMSG_PORT
	}
	d, _ := json.Marshal(regmsg)
	if bytes.Contains(d, []byte{split}) {
		return ERRREGMSG_CHAR
	}
	clientinstance.lker.Lock()
	defer clientinstance.lker.Unlock()
	if clientinstance.regdata != nil {
		return ERRREG
	}
	clientinstance.regdata = d
	for serveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		server.regdata = d
		if server.status == 3 {
			server.status = 4
			log.Info("[Discovery.client.RegisterSelf] register to server:", serveruniquename, "with data:", common.Byte2str(clientinstance.regdata))
			server.peer.SendMessage(makeOnlineMsg("useless", clientinstance.regdata), server.starttime, true)
		}
		server.lker.Unlock()
	}
	return nil
}
func UnRegisterSelf() error {
	if clientinstance == nil {
		return ERRINIT
	}
	clientinstance.lker.Lock()
	if clientinstance.status == 0 {
		clientinstance.lker.Unlock()
		return nil
	}
	clientinstance.status = 0
	for k, server := range clientinstance.servers {
		server.lker.Lock()
		server.status = 0
		server.regdata = nil
		delete(clientinstance.servers, k)
		server.lker.Unlock()
	}
	clientinstance.lker.Unlock()
	clientinstance.instance.Stop()
	return nil
}

func NoticeWebChanges(appname string) (chan struct{}, error) {
	if clientinstance == nil {
		return nil, ERRINIT
	}
	pullmsg := makePullMsg(appname)
	clientinstance.lker.RLock()
	for _, server := range clientinstance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(pullmsg, server.starttime, true)
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	clientinstance.nlker.Lock()
	defer clientinstance.nlker.Unlock()
	if _, ok := clientinstance.webnotices[appname]; !ok {
		clientinstance.webnotices[appname] = make(map[chan struct{}]struct{}, 10)
	}
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	clientinstance.webnotices[appname][ch] = struct{}{}
	return ch, nil
}

func NoticeRpcChanges(appname string) (chan struct{}, error) {
	if clientinstance == nil {
		return nil, ERRINIT
	}
	pullmsg := makePullMsg(appname)
	clientinstance.lker.RLock()
	for _, server := range clientinstance.servers {
		server.lker.Lock()
		if server.status >= 3 {
			server.peer.SendMessage(pullmsg, server.starttime, true)
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	clientinstance.nlker.Lock()
	defer clientinstance.nlker.Unlock()
	if _, ok := clientinstance.rpcnotices[appname]; !ok {
		clientinstance.rpcnotices[appname] = make(map[chan struct{}]struct{}, 10)
	}
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	clientinstance.rpcnotices[appname][ch] = struct{}{}
	return ch, nil
}

//first key:app addr
//second key:discovery addr
//value:addition data
func GetRpcInfos(appname string) (map[string]map[string]struct{}, []byte) {
	return getinfos(appname, 1)
}

//first key:app addr
//second key:discovery addr
//value:addition data
func GetWebInfos(appname string) (map[string]map[string]struct{}, []byte) {
	return getinfos(appname, 2)
}

//first key:app addr
//second key:discovery addr
//value:addition data
func getinfos(appname string, t int) (map[string]map[string]struct{}, []byte) {
	result := make(map[string]map[string]struct{}, 5)
	var resultaddition []byte
	clientinstance.lker.RLock()
	defer clientinstance.lker.RUnlock()
	for serveruniquename, server := range clientinstance.servers {
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
					result[addr] = make(map[string]struct{}, 5)
				}
				result[addr][serveruniquename] = struct{}{}
				resultaddition = app.regmsg.Addition
			}
		}
		server.lker.Unlock()
	}
	return result, resultaddition
}

func (c *DiscoveryClient) verifyfunc(ctx context.Context, serveruniquename string, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
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
	clientinstance.nlker.RLock()
	result := make(map[string]struct{}, 5)
	for k := range clientinstance.rpcnotices {
		result[k] = struct{}{}
	}
	for k := range clientinstance.webnotices {
		result[k] = struct{}{}
	}
	clientinstance.nlker.RUnlock()
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
		fmt.Println("offline")
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
