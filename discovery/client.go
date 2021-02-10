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

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRINIT        = errors.New("[Discovery.client] not init,call NewDiscoveryClient first")
	ERRREG         = errors.New("[Discovery.client] already registered self")
	ERRREGMSG_PORT = errors.New("[Discovery.client] reg message port conflict")
	ERRREGMSG_CHAR = errors.New("[Discovery.client] reg message contains illegal character '|'")
	ERRREGMSG_NIL  = errors.New("[Discovery.client] reg message empty")
)

//in this function,call UpdateDiscoveryServers() to update the discovery servers
type DiscoveryServerFinder func()

//serveruniquename = servername:ip:port
type DiscoveryClient struct {
	selfname   string
	verifydata []byte
	instance   *stream.Instance
	regmsg     []byte
	finder     DiscoveryServerFinder
	status     int //0-closing,1-working
	//other
	lker    *sync.RWMutex
	servers map[string]*servernode //key serveruniquename
	//key appname
	rpcnotices map[string]map[chan struct{}]struct{}
	webnotices map[string]map[chan struct{}]struct{}
	nlker      *sync.RWMutex

	appnodepool *sync.Pool
}

//appuniquename = appname:ip:port
type servernode struct {
	lker      *sync.Mutex
	peer      *stream.Peer
	starttime uint64
	allapps   map[string]map[string]*appnode //first key:appname,second key:appuniquename=appname:ip:port
	status    int                            //0-closing,1-start,2-verify,3-connected,4-preparing,5-registered
	regmsg    []byte
}

func (c *DiscoveryClient) getnode(appuniquename string, regdata []byte, regmsg *RegMsg) *appnode {
	node, ok := c.appnodepool.Get().(*appnode)
	if !ok {
		return &appnode{
			appuniquename: appuniquename,
			regdata:       regdata,
			regmsg:        regmsg,
		}
	}
	node.appuniquename = appuniquename
	node.regdata = regdata
	node.regmsg = regmsg
	return node
}

func (c *DiscoveryClient) putnode(n *appnode) {
	n.appuniquename = ""
	n.regdata = nil
	c.appnodepool.Put(n)
}

var clientinstance *DiscoveryClient

//this just start the client and sync the peers in the net
//this will not register self into the net
//please call the RegisterSelf() func to register self into the net
func NewDiscoveryClient(c *stream.InstanceConfig, vdata []byte, finder DiscoveryServerFinder) {
	if finder == nil {
		log.Error("[Discovery.client] finder nil")
		return
	}
	temp := &DiscoveryClient{
		selfname:    c.SelfName,
		verifydata:  vdata,
		finder:      finder,
		status:      1,
		lker:        &sync.RWMutex{},
		servers:     make(map[string]*servernode, 5),
		webnotices:  make(map[string]map[chan struct{}]struct{}, 5),
		rpcnotices:  make(map[string]map[chan struct{}]struct{}, 5),
		nlker:       &sync.RWMutex{},
		appnodepool: &sync.Pool{},
	}
	if !atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&clientinstance)), nil, unsafe.Pointer(temp)) {
		return
	}
	//tcp instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = clientinstance.verifyfunc
	dupc.Onlinefunc = clientinstance.onlinefunc
	dupc.Userdatafunc = clientinstance.userfunc
	dupc.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(&dupc)
	go finder()
}
func UpdateDiscoveryServers(serveraddrs []string) {
	clientinstance.lker.Lock()
	if clientinstance.status == 0 {
		clientinstance.lker.Unlock()
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
		if findex == -1 || len(saddr) == findex+1 {
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
				regmsg:    clientinstance.regmsg,
			}
			clientinstance.servers[saddr] = server
			go clientinstance.start(saddr[findex+1:], saddr[:findex])
		}
	}
	clientinstance.lker.Unlock()
}
func (c *DiscoveryClient) start(addr, servername string) {
	tempverifydata := common.Byte2str(clientinstance.verifydata) + "|" + servername
	if r := c.instance.StartTcpClient(addr, common.Str2byte(tempverifydata)); r == "" {
		c.lker.RLock()
		server, ok := c.servers[servername+":"+addr]
		if !ok {
			//server removed
			c.lker.Unlock()
			return
		}
		server.lker.Lock()
		c.lker.RUnlock()
		if server.status == 0 {
			server.lker.Unlock()
		} else {
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
	if clientinstance.regmsg != nil {
		clientinstance.lker.Unlock()
		return ERRREG
	}
	clientinstance.regmsg = d
	for _, server := range clientinstance.servers {
		server.lker.Lock()
		server.regmsg = d
		if server.status == 4 {
			server.peer.SendMessage(makeOnlineMsg("", clientinstance.regmsg), server.starttime, true)
		}
		server.lker.Unlock()
	}
	clientinstance.lker.Unlock()
	return nil
}
func UnRegisterSelf() error {
	if clientinstance == nil {
		return ERRINIT
	}
	clientinstance.lker.Lock()
	clientinstance.status = 0
	for k, server := range clientinstance.servers {
		server.lker.Lock()
		server.status = 0
		server.regmsg = nil
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
	c.lker.RUnlock()
	if server.peer != nil || server.status != 1 || server.starttime != 0 {
		server.lker.Unlock()
		return nil, false
	}
	server.status = 2
	server.lker.Unlock()
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
	c.lker.RUnlock()
	if server.peer != nil || server.status != 2 || server.starttime != 0 {
		p.Close()
		server.lker.Unlock()
		return
	}
	server.peer = p
	server.starttime = starttime
	server.status = 3
	p.SetData(unsafe.Pointer(server))
	//after online the first message is pull all registered peers
	p.SendMessage(makePullMsg(), starttime, true)
	server.lker.Unlock()
	log.Info("[Discovery.client.onlinefunc] server:", serveruniquename, "online")
	return
}
func (c *DiscoveryClient) userfunc(p *stream.Peer, serveruniquename string, origindata []byte, starttime uint64) {
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
		onlineapp, regdata, e := getOnlineMsg(data)
		if e != nil {
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "online message:", common.Byte2str(data), "broken")
			p.Close()
			return
		}
		regmsg := &RegMsg{}
		if e = json.Unmarshal(regdata, regmsg); e != nil || (regmsg.RpcPort == 0 && (regmsg.WebPort == 0 || regmsg.WebScheme == "")) {
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "online app:", onlineapp, "register message:", common.Byte2str(regdata), "broken")
			p.Close()
			return
		}
		appname := onlineapp[:strings.Index(onlineapp, ":")]
		node := c.getnode(onlineapp, regdata, regmsg)
		if _, ok := server.allapps[appname]; !ok {
			server.allapps[appname] = make(map[string]*appnode, 5)
		}
		server.allapps[appname][onlineapp] = node
		c.nlker.RLock()
		c.notice(onlineapp)
		c.nlker.RUnlock()
		if appname == c.selfname {
			log.Info("[Discovery.client.userfunc] registered on server:", serveruniquename)
		}
	case msgoffline:
		offlineapp, e := getOfflineMsg(data)
		if e != nil {
			//this is impossible
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "offline message:", common.Byte2str(data), "broken")
			p.Close()
			return
		}
		appname := offlineapp[:strings.Index(offlineapp, ":")]
		if _, ok := server.allapps[appname]; !ok {
			//this is impossible
			log.Error("[Discovery.client.userfunc] app:", offlineapp, "missing")
			p.SendMessage(makePullMsg(), server.starttime, true)
			return
		}
		node, ok := server.allapps[appname][offlineapp]
		if !ok {
			//this is impossible
			log.Error("[Discovery.client.userfunc] app:", offlineapp, "missing")
			p.SendMessage(makePullMsg(), server.starttime, true)
			return
		}
		delete(server.allapps[appname], offlineapp)
		c.putnode(node)
		if len(server.allapps[appname]) == 0 {
			delete(server.allapps, appname)
		}
		c.nlker.RLock()
		c.notice(offlineapp)
		c.nlker.RUnlock()
	case msgpush:
		all, e := getPushMsg(data)
		if e != nil {
			//this is impossible
			log.Error("[Discovery.client.userfunc] server:", serveruniquename, "push message:", common.Byte2str(data), "broken")
			p.Close()
			return
		}
		notices := make(map[string]struct{})
		for appname, oldappgroup := range server.allapps {
			for _, oldapp := range oldappgroup {
				if newmsg, ok := all[oldapp.appuniquename]; !ok {
					//delete
					delete(oldappgroup, oldapp.appuniquename)
					notices[oldapp.appuniquename] = struct{}{}
				} else if !bytes.Equal(oldapp.regdata, newmsg) {
					//this is impossible
					log.Error("[Discovery.client.userfunc] app:", oldapp.appuniquename,
						"regmsg changed from:", common.Byte2str(oldapp.regdata), "to:", common.Byte2str(newmsg))
					//replace
					regmsg := &RegMsg{}
					e = json.Unmarshal(newmsg, regmsg)
					if e != nil || (regmsg.RpcPort == 0 && (regmsg.WebPort == 0 || regmsg.WebScheme == "")) {
						//this is impossible
						log.Error("[Discovery.client.userfunc] server:", serveruniquename,
							"app:", oldapp.appuniquename, "regmsg:", common.Byte2str(newmsg), "broken")
						p.Close()
						return
					}
					oldapp.regdata = newmsg
					oldapp.regmsg = regmsg
					notices[oldapp.appuniquename] = struct{}{}
				}
			}
			if len(oldappgroup) == 0 {
				delete(server.allapps, appname)
			}
		}
		for newapp, newmsg := range all {
			regmsg := &RegMsg{}
			if e = json.Unmarshal(newmsg, regmsg); e != nil || (regmsg.RpcPort == 0 && (regmsg.WebPort == 0 || regmsg.WebScheme == "")) {
				//thie is impossible
				log.Error("[Discovery.client.userfunc] server:", serveruniquename, "app:", newapp, "regmsg:", common.Byte2str(newmsg), "broken")
				p.Close()
				return
			}
			appname := newapp[:strings.Index(newapp, ":")]
			if _, ok := server.allapps[appname]; !ok {
				//add
				server.allapps[appname] = make(map[string]*appnode, 5)
				server.allapps[appname][newapp] = c.getnode(newapp, newmsg, regmsg)
				notices[newapp] = struct{}{}
			} else if oldapp, ok := server.allapps[appname][newapp]; !ok {
				//add
				server.allapps[appname][newapp] = c.getnode(newapp, newmsg, regmsg)
				notices[newapp] = struct{}{}
			} else if !bytes.Equal(oldapp.regdata, newmsg) {
				//this is impossible
				log.Error("[Discovery.client.userfunc] server:", serveruniquename,
					"app:", newapp, "regmsg changed from:", common.Byte2str(oldapp.regdata), "to:", common.Byte2str(newmsg))
				//replace
				oldapp.regdata = newmsg
				oldapp.regmsg = regmsg
				notices[newapp] = struct{}{}
			}
		}
		//notice
		c.nlker.RLock()
		for app := range notices {
			c.notice(app)
		}
		c.nlker.RUnlock()
		if server.status == 3 {
			log.Info("[Discovery.client.userfunc] prepared on server:", serveruniquename)
			if len(server.regmsg) != 0 {
				fmt.Printf("regmsg:%s\n", server.regmsg)
				server.status = 5
				server.peer.SendMessage(makeOnlineMsg("", server.regmsg), server.starttime, true)
			} else {
				fmt.Println("regmsg:nil")
				server.status = 4
			}
		}
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
	for _, appgroup := range server.allapps {
		for _, app := range appgroup {
			clientinstance.notice(app.appuniquename)
		}
	}
	c.nlker.Unlock()
	server.allapps = make(map[string]map[string]*appnode, 5)
	if server.status == 0 {
		server.lker.Unlock()
	} else {
		server.status = 1
		server.lker.Unlock()
		time.Sleep(100 * time.Millisecond)
		index := strings.Index(serveruniquename, ":")
		go c.start(serveruniquename[index+1:], serveruniquename[:index])
	}
}
func (c *DiscoveryClient) notice(appuniquename string) {
	appname := appuniquename[:strings.Index(appuniquename, ":")]
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
