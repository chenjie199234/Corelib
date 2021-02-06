package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/common"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRCINIT        = fmt.Errorf("[Discovery.client]not init,call NewDiscoveryClient first")
	ERRCREG         = fmt.Errorf("[Discovery.client]already registered self")
	ERRCNOTICE      = fmt.Errorf("[Discovery.client]already noticed")
	ERRCREGMSG_PORT = fmt.Errorf("[Discovery.client]reg message port conflict")
	ERRCREGMSG_CHAR = fmt.Errorf("[Discovery.client]reg message contains illegal character '|'")
	ERRCREGMSG_NIL  = fmt.Errorf("[Discovery.client]reg message empty")
)

//serveruniquename = servername:ip:port
type discoveryclient struct {
	c          *stream.InstanceConfig
	httpclient *http.Client //httpclient to get discovery server addrs

	lker       *sync.RWMutex
	verifydata []byte
	servers    map[string]*servernode //key serveruniquename
	instance   *stream.Instance
	regmsg     []byte
	canreg     bool
	stopch     chan struct{}
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
	status    int                            //0-idle,1-start,2-verify,3-connected,4-preparing,5-registered
}

func (c *discoveryclient) getnode(appuniquename string, regdata []byte, regmsg *RegMsg) *appnode {
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

func (c *discoveryclient) putnode(n *appnode) {
	n.appuniquename = ""
	n.regdata = nil
	c.appnodepool.Put(n)
}

var clientinstance *discoveryclient

//this just start the client and sync the peers in the net
//this will not register self into the net
//please call the RegisterSelf() func to register self into the net
func NewDiscoveryClient(c *stream.InstanceConfig, vdata []byte, url string) {
	temp := &discoveryclient{
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
		lker:        &sync.RWMutex{},
		verifydata:  vdata,
		servers:     make(map[string]*servernode, 5),
		stopch:      make(chan struct{}),
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
	clientinstance.c = &dupc
	clientinstance.instance = stream.NewInstance(&dupc)

	clientinstance.updateserver(url)
	go func() {
		tker := time.NewTicker(time.Second)
		for {
			select {
			case <-clientinstance.stopch:
				tker.Stop()
				for len(tker.C) > 0 {
					<-tker.C
				}
				clientinstance.instance.Stop()
				clientinstance.servers = make(map[string]*servernode, 0)
				return
			default:
				select {
				case <-clientinstance.stopch:
					tker.Stop()
					for len(tker.C) > 0 {
						<-tker.C
					}
					clientinstance.instance.Stop()
					clientinstance.servers = make(map[string]*servernode, 0)
					return
				case _, ok := <-tker.C:
					if ok {
						clientinstance.updateserver(url)
					}
				}
			}
		}
	}()
}
func RegisterSelf(regmsg *RegMsg) error {
	if clientinstance == nil {
		return ERRCINIT
	}
	if regmsg == nil {
		return ERRCREGMSG_NIL
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
		return ERRCREGMSG_NIL
	}
	if len(temp) != count {
		return ERRCREGMSG_PORT
	}
	d, _ := json.Marshal(regmsg)
	if bytes.Contains(d, []byte{split}) {
		return ERRCREGMSG_CHAR
	}
	clientinstance.lker.Lock()
	if clientinstance.canreg {
		clientinstance.lker.Unlock()
		return ERRCREG
	}
	clientinstance.regmsg = d
	clientinstance.canreg = true
	clientinstance.lker.Unlock()
	return nil
}
func UnRegisterSelf() error {
	if clientinstance == nil {
		return ERRCINIT
	}
	select {
	case clientinstance.stopch <- struct{}{}:
	default:
	}
	return nil
}

func NoticeWebChanges(appname string) (chan struct{}, error) {
	if clientinstance == nil {
		return nil, ERRCINIT
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
		return nil, ERRCINIT
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
					addr = fmt.Sprintf("%s:%d", app.regmsg.RpcIp, app.regmsg.RpcPort)
				case 2:
					if app.regmsg.WebPort == 0 || app.regmsg.WebScheme == "" {
						continue
					}
					addr = fmt.Sprintf("%s://%s:%d", app.regmsg.WebScheme, app.regmsg.WebIp, app.regmsg.WebPort)
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

func (c *discoveryclient) updateserver(url string) {
	//get server addrs
	resp, e := c.httpclient.Get(url)
	if e != nil {
		fmt.Printf("[Discovery.client.updateserver]get discovery server addrs error:%s\n", e)
		return
	}
	data, e := ioutil.ReadAll(resp.Body)
	if e != nil {
		fmt.Printf("[Discovery.client.updateserver]read discovery server addrs from response error:%s\n", e)
		return
	}
	//elemt is serveruniquename = servername:ip:port
	serveraddrs := make([]string, 0)
	if e := json.Unmarshal(data, &serveraddrs); e != nil {
		fmt.Printf("[Discovery.client.updateserver]read discovery server addrs from response data:%s format error:%s\n", data, e)
		return
	}
	c.lker.Lock()
	//delete offline server
	for serveruniquename, server := range c.servers {
		find := false
		for _, saddr := range serveraddrs {
			if saddr == serveruniquename {
				find = true
				break
			}
		}
		server.lker.Lock()
		if !find {
			delete(c.servers, serveruniquename)
			if server.peer != nil {
				server.peer.Close()
			}
			//notice
			c.nlker.Lock()
			for _, appgroup := range server.allapps {
				for _, app := range appgroup {
					c.notice(app.appuniquename)
					c.putnode(app)
				}
			}
			c.nlker.Unlock()
		} else if c.canreg && server.status == 4 {
			server.status = 5
			server.peer.SendMessage(makeOnlineMsg("", c.regmsg), server.starttime, true)
		}
		server.lker.Unlock()
	}
	//online new server or reconnect to offline server
	for _, saddr := range serveraddrs {
		//check saddr
		findex := strings.Index(saddr, ":")
		if findex == -1 || len(saddr) == findex+1 {
			fmt.Printf("[Discovery.client.updateserver]server addr:%s format error\n", saddr)
			continue
		}
		if _, e := net.ResolveTCPAddr("tcp", saddr[findex+1:]); e != nil {
			fmt.Printf("[Discovery.client.updateserver]server addr:%s tcp addr error\n", saddr)
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
				status:    0,
			}
			c.servers[saddr] = server
		}
		server.lker.Lock()
		if server.status == 0 {
			server.status = 1
			go func(saddr string, findex int) {
				tempverifydata := common.Byte2str(c.verifydata) + "|" + saddr[:findex]
				if r := c.instance.StartTcpClient(saddr[findex+1:], common.Str2byte(tempverifydata)); r == "" {
					c.lker.RLock()
					server, ok := c.servers[saddr]
					if !ok {
						//discovery server removed
						c.lker.RUnlock()
						return
					}
					server.lker.Lock()
					c.lker.RUnlock()
					server.status = 0
					server.lker.Unlock()
				}
			}(saddr, findex)
		}
		server.lker.Unlock()
	}
	c.lker.Unlock()
}

func (c *discoveryclient) verifyfunc(ctx context.Context, serveruniquename string, peerVerifyData []byte) ([]byte, bool) {
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
	if server.peer != nil || server.starttime != 0 || server.status != 1 {
		server.lker.Unlock()
		//this is impossible
		fmt.Printf("[Discovery.client.verifyfunc.impossible]server:%s conflict\n", serveruniquename)
		return nil, false
	}
	server.status = 2
	server.lker.Unlock()
	return nil, true
}
func (c *discoveryclient) onlinefunc(p *stream.Peer, serveruniquename string, starttime uint64) {
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
	if server.peer != nil || server.starttime != 0 || server.status != 2 {
		//this is impossible
		p.Close()
		server.lker.Unlock()
		fmt.Printf("[Discovery.client.onlinefunc.impossible]server:%s conflict\n", serveruniquename)
		return
	}
	server.status = 3
	server.peer = p
	server.starttime = starttime
	p.SetData(unsafe.Pointer(server))
	//after online the first message is pull all registered peers
	p.SendMessage(makePullMsg(), starttime, true)
	server.lker.Unlock()
}
func (c *discoveryclient) userfunc(p *stream.Peer, serveruniquename string, data []byte, starttime uint64) {
	server := (*servernode)(p.GetData())
	server.lker.Lock()
	defer server.lker.Unlock()
	switch data[0] {
	case msgonline:
		onlineapp, regdata, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.msgonline.impossible]server:%s online message:%s broken\n", serveruniquename, data)
			p.Close()
			return
		}
		regmsg := &RegMsg{}
		if e = json.Unmarshal(regdata, regmsg); e != nil || (regmsg.RpcPort == 0 && (regmsg.WebPort == 0 || regmsg.WebScheme == "")) {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.msgonline.impossible]server:%s online app:%s register message:%s broken\n", serveruniquename, onlineapp, regdata)
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
		if onlineapp[:strings.Index(onlineapp, ":")] == c.c.SelfName && bytes.Equal(regdata, c.regmsg) {
			fmt.Printf("[Discovery.client.userfunc.msgonline]self registered on discovery server:%s\n", serveruniquename)
		}
	case msgoffline:
		offlineapp, e := getOfflineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.msgoffline.impossible]server:%s offline message:%s broken\n", serveruniquename, data)
			p.Close()
			return
		}
		appname := offlineapp[:strings.Index(offlineapp, ":")]
		if _, ok := server.allapps[appname]; !ok {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.msgoffline.impossible]app:%s missing\n", offlineapp)
			p.SendMessage(makePullMsg(), server.starttime, true)
			return
		}
		node, ok := server.allapps[appname][offlineapp]
		if !ok {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.msgoffline.impossible]app:%s missing\n", offlineapp)
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
			fmt.Printf("[Discovery.client.userfunc.msgpush.impossible]server:%s push message:%d broken\n", serveruniquename, data)
			p.Close()
			return
		}
		if server.status == 3 {
			server.status = 4
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
					fmt.Printf("[Discovery.client.userfunc.msgpush.impossible]app:%s regmsg changed from:%s to:%s\n", oldapp.appuniquename, oldapp.regdata, newmsg)
					//replace
					regmsg := &RegMsg{}
					e = json.Unmarshal(newmsg, regmsg)
					if e != nil || (regmsg.RpcPort == 0 && (regmsg.WebPort == 0 || regmsg.WebScheme == "")) {
						//this is impossible
						fmt.Printf("[Discovery.client.userfunc.msgpush.impossible]server:%s app:%s regmsg:%s broken\n",
							serveruniquename, oldapp.appuniquename, newmsg)
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
				fmt.Printf("[Discovery.client.userfunc.msgpush.impossible]server:%s app:%s regmsg:%s broken\n", serveruniquename, newapp, newmsg)
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
				fmt.Printf("[Discovery.client.userfunc.msgpush.impossible]server:%s app:%s regmsg changed from:%s to:%s\n",
					serveruniquename, newapp, oldapp.regdata, newmsg)
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
		if server.status == 4 {
			fmt.Printf("[Discovery.client.userfunc]self prepared on discovery server:%s\n", serveruniquename)
		}
	default:
		fmt.Printf("[Discovery.client.userfunc.impossible]unknown message type")
		p.Close()
	}
}
func (c *discoveryclient) offlinefunc(p *stream.Peer, serveruniquename string, starttime uint64) {
	//this is just disconnect to the discovery server
	//keep the apps register info
	fmt.Printf("[Discovery.client.offlinefunc]self unregistered on discovery server:%s\n", serveruniquename)
	server := (*servernode)(p.GetData())
	server.lker.Lock()
	server.peer = nil
	server.starttime = 0
	server.status = 0
	server.lker.Unlock()
}
func (c *discoveryclient) notice(appuniquename string) {
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
