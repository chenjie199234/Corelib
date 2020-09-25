package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/chenjie199234/Corelib/hashtree"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRCINIT        = fmt.Errorf("[Discovery.client]not init,call NewDiscoveryClient first")
	ERRCREG         = fmt.Errorf("[Discovery.client]already registered self")
	ERRCNOTICE      = fmt.Errorf("[Discovery.client]already exist")
	ERRCREGMSG_PORT = fmt.Errorf("[Discovery.client]reg message port conflict")
	ERRCREGMSG_CHAR = fmt.Errorf("[Discovery.client]reg message contains illegal character '|'")
	ERRCREGMSG_NIL  = fmt.Errorf("[Discovery.client]reg message empty")
)

//serveruniquename = discoveryservername:addr
type client struct {
	lker       *sync.RWMutex
	servers    map[string]*servernode //key serveruniquename
	verifydata []byte
	instance   *stream.Instance
	httpclient *http.Client
	regmsg     *RegMsg
	//key appname
	grpcnotices map[string]chan *NoticeMsg
	httpnotices map[string]chan *NoticeMsg
	tcpnotices  map[string]chan *NoticeMsg
	webnotices  map[string]chan *NoticeMsg
	nlker       *sync.RWMutex
	canregister bool
}

//clientuniquename = appname:addr
type servernode struct {
	lker       *sync.Mutex
	peer       *stream.Peer
	uniqueid   uint64
	htree      *hashtree.Hashtree
	allclients map[string]*clientinfo //key clientuniquename
	status     int                    //1 connected,2 preparing,3 registered
}

//clientuniquename = appname:addr
type clientinfo struct {
	clientuniquename string
	regdata          []byte
}

var clientinstance *client

//this just start the client and sync the peers in the net
//this will not register self into the net
//please call the RegisterSelf() func to register self into the net
func NewDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, url string) {
	if clientinstance != nil {
		return
	}
	clientinstance = &client{
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*servernode, 5),
		verifydata: vdata,
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
		grpcnotices: make(map[string]chan *NoticeMsg, 5),
		httpnotices: make(map[string]chan *NoticeMsg, 5),
		tcpnotices:  make(map[string]chan *NoticeMsg, 5),
		webnotices:  make(map[string]chan *NoticeMsg, 5),
		nlker:       &sync.RWMutex{},
	}
	//tcp instance
	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)

	clientinstance.updateserver(cc, url)
	tker := time.NewTicker(time.Second)
	go func() {
		for {
			<-tker.C
			clientinstance.updateserver(cc, url)
		}
	}()
}
func RegisterSelf(regmsg *RegMsg) error {
	if clientinstance == nil {
		return ERRSINIT
	}
	if regmsg == nil {
		return ERRCREGMSG_NIL
	}
	temp := make(map[int]struct{})
	count := 0
	if regmsg.GrpcPort != 0 {
		temp[regmsg.GrpcPort] = struct{}{}
		count++
	}
	if regmsg.HttpPort != 0 {
		temp[regmsg.HttpPort] = struct{}{}
		count++
	}
	if regmsg.TcpPort != 0 {
		temp[regmsg.TcpPort] = struct{}{}
		count++
	}
	if regmsg.WebSockPort != 0 {
		temp[regmsg.WebSockPort] = struct{}{}
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
	if clientinstance.canregister {
		clientinstance.lker.Unlock()
		return ERRCREG
	}
	clientinstance.regmsg = regmsg
	clientinstance.canregister = true
	clientinstance.lker.Unlock()
	return nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func GrpcNotice(appname string) (map[string]map[string][]byte, chan *NoticeMsg, error) {
	if clientinstance == nil {
		return nil, nil, ERRCINIT
	}
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.grpcnotices[appname]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERRCNOTICE
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.grpcnotices[appname] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string][]byte, 5)
	clientinstance.lker.RLock()
	for serveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allclients {
			if peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] != appname {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.GrpcNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.clientuniquename, peer.regdata, serveruniquename)
				continue
			}
			if msg.GrpcPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.GrpcIp, msg.GrpcPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string][]byte, 5)
			}
			result[addr][serveruniquename] = msg.Addition
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func HttpNotice(appname string) (map[string]map[string][]byte, chan *NoticeMsg, error) {
	if clientinstance == nil {
		return nil, nil, ERRCINIT
	}
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.httpnotices[appname]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERRCNOTICE
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.httpnotices[appname] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string][]byte, 5)
	clientinstance.lker.RLock()
	for serveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allclients {
			if peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] != appname {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.HttpNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.clientuniquename, peer.regdata, serveruniquename)
				continue
			}
			if msg.HttpPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.HttpIp, msg.HttpPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string][]byte, 5)
			}
			result[addr][serveruniquename] = msg.Addition
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func TcpNotice(appname string) (map[string]map[string][]byte, chan *NoticeMsg, error) {
	if clientinstance == nil {
		return nil, nil, ERRCINIT
	}
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.tcpnotices[appname]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERRCNOTICE
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.tcpnotices[appname] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string][]byte, 5)
	clientinstance.lker.RLock()
	for serveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allclients {
			if peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] != appname {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.TcpNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.clientuniquename, peer.regdata, serveruniquename)
				continue
			}
			if msg.TcpPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.TcpIp, msg.TcpPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string][]byte, 5)
			}
			result[addr][serveruniquename] = msg.Addition
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func WebSocketNotice(peername string) (map[string]map[string][]byte, chan *NoticeMsg, error) {
	if clientinstance == nil {
		return nil, nil, ERRCINIT
	}
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.webnotices[peername]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERRCNOTICE
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.webnotices[peername] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string][]byte, 5)
	clientinstance.lker.RLock()
	for serveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allclients {
			if peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] != peername {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.WebSocketNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.clientuniquename, peer.regdata, serveruniquename)
				continue
			}
			if msg.WebSockPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.WebSockIp, msg.WebSockPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string][]byte, 5)
			}
			result[addr][serveruniquename] = msg.Addition
		}
		server.lker.Unlock()
	}
	clientinstance.lker.RUnlock()
	return result, ch, nil
}

func (c *client) updateserver(cc *stream.TcpConfig, url string) {
	//get server addrs
	resp, e := c.httpclient.Get(url)
	if e != nil {
		fmt.Printf("[Discovery.client.updateserver]get discovery server addr error:%s\n", e)
		return
	}
	data, e := ioutil.ReadAll(resp.Body)
	if e != nil {
		fmt.Printf("[Discovery.client.updateserver]read response data error:%s\n", e)
		return
	}
	serveraddrs := make([]string, 0)
	if e := json.Unmarshal(data, &serveraddrs); e != nil {
		fmt.Printf("[Discovery.client.updateserver]response data:%s format error:%s\n", data, e)
		return
	}
	c.lker.Lock()
	//delete offline server
	for k, v := range c.servers {
		find := false
		for _, saddr := range serveraddrs {
			if saddr == k {
				find = true
				break
			}
		}
		if !find {
			delete(c.servers, k)
			v.peer.Close(v.uniqueid)
		} else if c.canregister && v.status == 2 {
			regmsg, _ := json.Marshal(c.regmsg)
			v.peer.SendMessage(makeOnlineMsg("", regmsg, nil), v.uniqueid)
			v.status = 3
		}
	}
	//online new server or reconnect to offline server
	for _, saddr := range serveraddrs {
		find := false
		for k, v := range c.servers {
			if k == saddr {
				if v.uniqueid != 0 {
					find = true
				}
				break
			}
		}
		//discovery server not registered or discovery server offline
		if !find {
			findex := strings.Index(saddr, ":")
			if findex == -1 || len(saddr) == findex+1 {
				fmt.Printf("[Discovery.client.updateserver]server addr:%s format error\n", saddr)
				continue
			}
			if _, e := net.ResolveTCPAddr("tcp", saddr[findex+1:]); e != nil {
				fmt.Printf("[Discovery.client.updateserver]server addr:%s tcp addr error\n", saddr)
				continue
			}
			c.servers[saddr] = &servernode{
				lker:       &sync.Mutex{},
				peer:       nil,
				uniqueid:   0,
				htree:      hashtree.New(10, 3),
				allclients: make(map[string]*clientinfo, 5),
			}
			go c.instance.StartTcpClient(cc, saddr[findex+1:], c.verifydata)
		}
	}
	c.lker.Unlock()
}
func (c *client) verifyfunc(ctx context.Context, serveruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, serveruniquename string, uniqueid uint64) {
	c.lker.RLock()
	server, ok := c.servers[serveruniquename]
	c.lker.RUnlock()
	if !ok {
		//this discovery server had already been unregistered
		return
	}
	server.lker.Lock()
	server.peer = p
	server.uniqueid = uniqueid
	server.status = 1
	p.SetData(unsafe.Pointer(server), uniqueid)
	//after online the first message is pull all registered peers
	p.SendMessage(makePullMsg(), uniqueid)
	server.lker.Unlock()
	return
}
func (c *client) userfunc(p *stream.Peer, serveruniquename string, uniqueid uint64, data []byte) {
	tempserver, e := p.GetData(uniqueid)
	if e != nil {
		//server closed
		return
	}
	server := (*servernode)(tempserver)
	server.lker.Lock()
	defer server.lker.Unlock()
	switch data[0] {
	case msgonline:
		onlinepeer, regmsg, newhash, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]online peer:%s message:%s broken from discovery server:%s\n", onlinepeer, data, serveruniquename)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(onlinepeer, uint64(server.htree.GetLeavesNum())))
		templeafdata, _ := server.htree.GetLeafValue(leafindex)
		if templeafdata == nil {
			leafdata := make([]*clientinfo, 0, 5)
			temppeer := &clientinfo{
				clientuniquename: onlinepeer,
				regdata:          regmsg,
			}
			leafdata = append(leafdata, temppeer)
			server.allclients[onlinepeer] = temppeer
			server.htree.SetSingleLeafHash(leafindex, str2byte(onlinepeer[:strings.Index(onlinepeer, ":")]+byte2str(regmsg)))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		} else {
			leafdata := *(*[]*clientinfo)(templeafdata)
			temppeer := &clientinfo{
				clientuniquename: onlinepeer,
				regdata:          regmsg,
			}
			leafdata = append(leafdata, temppeer)
			server.allclients[onlinepeer] = temppeer
			sort.Slice(leafdata, func(i, j int) bool {
				return leafdata[i].clientuniquename < leafdata[j].clientuniquename
			})
			all := make([]string, len(leafdata))
			for i, peer := range leafdata {
				all[i] = peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] + byte2str(peer.regdata)
			}
			server.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]data from discovery server:%s conflict\n", serveruniquename)
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.nlker.RLock()
			c.notice(onlinepeer, regmsg, true, serveruniquename)
			c.nlker.RUnlock()
		}
	case msgoffline:
		offlinepeer, newhash, e := getOfflineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]offline peer:%s message:%s broken from discovery server:%s\n", offlinepeer, data, serveruniquename)
			p.Close(uniqueid)
			return
		}
		delete(server.allclients, offlinepeer)
		leafindex := int(bkdrhash(offlinepeer, uint64(server.htree.GetLeavesNum())))
		templeafdata, _ := server.htree.GetLeafValue(leafindex)
		leafdata := *(*[]*clientinfo)(templeafdata)
		var regmsg []byte
		for i, peer := range leafdata {
			if peer.clientuniquename == offlinepeer {
				leafdata = append(leafdata[:i], leafdata[i+1:]...)
				regmsg = peer.regdata
				break
			}
		}
		if len(leafdata) == 0 {
			server.htree.SetSingleLeafHash(leafindex, nil)
			server.htree.SetSingleLeafValue(leafindex, nil)
		} else {
			all := make([]string, len(leafdata))
			for i, peer := range leafdata {
				all[i] = peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] + byte2str(peer.regdata)
			}
			server.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]data from discovery server:%s conflict\n", serveruniquename)
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.nlker.RLock()
			c.notice(offlinepeer, regmsg, false, serveruniquename)
			c.nlker.RUnlock()
		}
	case msgpush:
		all, e := getPushMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]push message:%d broken from discovery server:%s\n", data, serveruniquename)
			p.Close(uniqueid)
			return
		}
		server.status = 2
		deleted := make(map[int]map[string][]byte, 5)
		added := make(map[int]map[string][]byte, 5)
		replace := make(map[int]map[string][]byte)
		updateleaveshash := make(map[int][]byte, 5)
		updateleavesvalue := make(map[int]unsafe.Pointer, 5)
		for _, oldpeer := range server.allclients {
			if newmsg, ok := all[oldpeer.clientuniquename]; !ok {
				//delete
				leafindex := int(bkdrhash(oldpeer.clientuniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := deleted[leafindex]; !ok {
					deleted[leafindex] = make(map[string][]byte, 5)
				}
				deleted[leafindex][oldpeer.clientuniquename] = oldpeer.regdata
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			} else if !bytes.Equal(oldpeer.regdata, newmsg) {
				//replace
				leafindex := int(bkdrhash(oldpeer.clientuniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := replace[leafindex]; !ok {
					replace[leafindex] = make(map[string][]byte, 5)
				}
				replace[leafindex][oldpeer.clientuniquename] = newmsg
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			}
		}
		for newpeer, newmsg := range all {
			if oldpeer, ok := server.allclients[newpeer]; !ok {
				//add
				leafindex := int(bkdrhash(newpeer, uint64(server.htree.GetLeavesNum())))
				if _, ok := added[leafindex]; !ok {
					added[leafindex] = make(map[string][]byte, 5)
				}
				added[leafindex][newpeer] = newmsg
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			} else if !bytes.Equal(oldpeer.regdata, newmsg) {
				//this is impossible
				fmt.Printf("[Discovery.client.userfunc.impossible]peer:%s regmsg conflict", oldpeer.clientuniquename)
				//replace
				leafindex := int(bkdrhash(newpeer, uint64(server.htree.GetLeavesNum())))
				if _, ok := replace[leafindex]; !ok {
					replace[leafindex] = make(map[string][]byte, 5)
				}
				replace[leafindex][newpeer] = newmsg
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			}
		}
		for leafindex := range updateleaveshash {
			templeafdata, _ := server.htree.GetLeafValue(leafindex)
			var leafdata []*clientinfo
			if templeafdata == nil {
				leafdata = make([]*clientinfo, 0, 5)
			} else {
				leafdata = *(*[]*clientinfo)(templeafdata)
			}
			//add
			for newpeer, newmsg := range added[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.clientuniquename == newpeer {
						//this is impossible
						find = true
						oldpeer.regdata = newmsg
						server.allclients[newpeer] = oldpeer
						c.nlker.RLock()
						c.notice(newpeer, oldpeer.regdata, false, serveruniquename)
						c.notice(newpeer, newmsg, true, serveruniquename)
						c.nlker.RUnlock()
						break
					}
				}
				if !find {
					newnode := &clientinfo{clientuniquename: newpeer, regdata: newmsg}
					leafdata = append(leafdata, newnode)
					server.allclients[newpeer] = newnode
					c.nlker.RLock()
					c.notice(newpeer, newmsg, true, serveruniquename)
					c.nlker.RUnlock()
				}
			}
			//replace
			for newpeer, newmsg := range replace[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.clientuniquename == newpeer {
						find = true
						oldpeer.regdata = newmsg
						server.allclients[newpeer] = oldpeer
						c.nlker.RLock()
						c.notice(newpeer, oldpeer.regdata, false, serveruniquename)
						c.notice(newpeer, newmsg, true, serveruniquename)
						c.nlker.RUnlock()
						break
					}
				}
				if !find {
					//this is impossible
					server.allclients[newpeer].regdata = newmsg
					leafdata = append(leafdata, server.allclients[newpeer])
					c.nlker.RLock()
					c.notice(newpeer, newmsg, true, serveruniquename)
					c.nlker.RUnlock()
				}
			}
			//deleted
			pos := len(leafdata) - 1
			for oldpeer, oldmsg := range deleted[leafindex] {
				delete(server.allclients, oldpeer)
				for i, v := range leafdata {
					if v.clientuniquename == oldpeer {
						if i != pos {
							leafdata[i], leafdata[pos] = leafdata[pos], leafdata[i]
						}
						pos--
						break
					}
				}
				c.nlker.RLock()
				c.notice(oldpeer, oldmsg, false, serveruniquename)
				c.nlker.RUnlock()
			}
			leafdata = leafdata[:pos+1]
			if len(leafdata) == 0 {
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			} else {
				sort.Slice(leafdata, func(i, j int) bool {
					return leafdata[i].clientuniquename < leafdata[j].clientuniquename
				})
				all := make([]string, len(leafdata))
				for i, peer := range leafdata {
					all[i] = peer.clientuniquename[:strings.Index(peer.clientuniquename, ":")] + byte2str(peer.regdata)
				}
				updateleaveshash[leafindex] = str2byte(strings.Join(all, ""))
				updateleavesvalue[leafindex] = unsafe.Pointer(&leafdata)
			}
		}
		server.htree.SetMultiLeavesHash(updateleaveshash)
		server.htree.SetMultiLeavesValue(updateleavesvalue)
	default:
		fmt.Printf("[Discovery.client.userfunc.impossible]unknown message type")
		p.Close(uniqueid)
	}
}
func (c *client) offlinefunc(p *stream.Peer, serveruniquename string, uniqueid uint64) {
	tempserver, e := p.GetData(uniqueid)
	if e != nil {
		//server closed,this is impossible
		fmt.Printf("[Discovery.client.offlinefunc.impossible]server offline before offlinefunc called\n")
		return
	}
	server := (*servernode)(tempserver)
	server.lker.Lock()
	defer server.lker.Unlock()
	server.peer = nil
	server.uniqueid = 0
	server.htree.Reset()
	c.nlker.RLock()
	for _, peer := range server.allclients {
		c.notice(peer.clientuniquename, peer.regdata, false, serveruniquename)
	}
	c.nlker.RUnlock()
	server.allclients = make(map[string]*clientinfo)
	server.status = 0
}
func (c *client) notice(clientuniquename string, regmsg []byte, status bool, servername string) {
	msg := &RegMsg{}
	e := json.Unmarshal(regmsg, msg)
	if e != nil {
		//this is impossible
		fmt.Printf("[Discovery.client.notice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", clientuniquename, regmsg, servername)
		return
	}
	appname := clientuniquename[:strings.Index(clientuniquename, ":")]
	if notice, ok := c.grpcnotices[appname]; ok && msg.GrpcPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.GrpcIp, msg.GrpcPort),
			Status:          status,
			DiscoveryServer: servername,
			Addition:        msg.Addition,
		}
	}
	if notice, ok := c.httpnotices[appname]; ok && msg.HttpPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.HttpIp, msg.HttpPort),
			Status:          status,
			DiscoveryServer: servername,
			Addition:        msg.Addition,
		}
	}
	if notice, ok := c.tcpnotices[appname]; ok && msg.TcpPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.TcpIp, msg.TcpPort),
			Status:          status,
			DiscoveryServer: servername,
			Addition:        msg.Addition,
		}
	}
	if notice, ok := c.webnotices[appname]; ok && msg.WebSockPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.WebSockIp, msg.WebSockPort),
			Status:          status,
			DiscoveryServer: servername,
			Addition:        msg.Addition,
		}
	}
}
