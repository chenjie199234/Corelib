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
	ERRSTARTED = fmt.Errorf("discovery client already started,can't register new notice after started")
	ERREXISTS  = fmt.Errorf("already exists this peer's notice")
)

type client struct {
	slker      *sync.RWMutex
	servers    map[string]*servernode
	verifydata []byte
	instance   *stream.Instance
	httpclient *http.Client
	regmsg     *RegMsg
	//key peername(not peeruniquename)
	grpcnotices map[string]chan *NoticeMsg
	httpnotices map[string]chan *NoticeMsg
	tcpnotices  map[string]chan *NoticeMsg
	webnotices  map[string]chan *NoticeMsg
	nlker       *sync.RWMutex
	canregister bool
}

type servernode struct {
	lker     *sync.Mutex
	peer     *stream.Peer
	uniqueid uint64
	htree    *hashtree.Hashtree
	allpeers map[string]*peerinfo //key peeruniquename
	status   int                  //1 connected,2 preparing,3 registered
}
type peerinfo struct {
	peeruniquename string
	regdata        []byte
}

var clientinstance *client

//this just start the client and sync the peers in the net
//this will not register self into the net
//please call the RegisterSelf() func to register self into the net
func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, regmsg *RegMsg, url string) {
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
	if count == 0 || len(temp) != count {
		panic("[Discovery.client.StartDiscoveryClient]regmsg port conflict")
	}
	d, _ := json.Marshal(regmsg)
	if bytes.Contains(d, []byte{sPLIT}) {
		panic("[Discovery.client.StartDiscoveryClient]regmsg contains illegal character '|'")
	}
	clientinstance = &client{
		slker:      &sync.RWMutex{},
		servers:    make(map[string]*servernode, 5),
		verifydata: vdata,
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
		regmsg:      regmsg,
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

	go clientinstance.updateserver(cc, url)
}
func RegisterSelf() {
	clientinstance.canregister = true
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func GrpcNotice(peername string) (map[string]map[string]struct{}, chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.grpcnotices[peername]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.grpcnotices[peername] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string]struct{}, 5)
	clientinstance.slker.RLock()
	for discoveryserveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allpeers {
			if peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] != peername {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.GrpcNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.peeruniquename, peer.regdata, discoveryserveruniquename)
				continue
			}
			if msg.GrpcPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.GrpcIp, msg.GrpcPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string]struct{}, 5)
			}
			result[addr][discoveryserveruniquename] = struct{}{}
		}
		server.lker.Unlock()
	}
	clientinstance.slker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func HttpNotice(peername string) (map[string]map[string]struct{}, chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.httpnotices[peername]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.httpnotices[peername] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string]struct{}, 5)
	clientinstance.slker.RLock()
	for discoveryserveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allpeers {
			if peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] != peername {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.HttpNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.peeruniquename, peer.regdata, discoveryserveruniquename)
				continue
			}
			if msg.HttpPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.HttpIp, msg.HttpPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string]struct{}, 5)
			}
			result[addr][discoveryserveruniquename] = struct{}{}
		}
		server.lker.Unlock()
	}
	clientinstance.slker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func TcpNotice(peername string) (map[string]map[string]struct{}, chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.tcpnotices[peername]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.tcpnotices[peername] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string]struct{}, 5)
	clientinstance.slker.RLock()
	for discoveryserveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allpeers {
			if peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] != peername {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.TcpNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.peeruniquename, peer.regdata, discoveryserveruniquename)
				continue
			}
			if msg.TcpPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.TcpIp, msg.TcpPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string]struct{}, 5)
			}
			result[addr][discoveryserveruniquename] = struct{}{}
		}
		server.lker.Unlock()
	}
	clientinstance.slker.RUnlock()
	return result, ch, nil
}

//first return:key addr,value discovery servers
//second return:peer update notice channel
func WebSocketNotice(peername string) (map[string]map[string]struct{}, chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if _, ok := clientinstance.webnotices[peername]; ok {
		clientinstance.nlker.Unlock()
		return nil, nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 1024)
	clientinstance.webnotices[peername] = ch
	clientinstance.nlker.Unlock()
	result := make(map[string]map[string]struct{}, 5)
	clientinstance.slker.RLock()
	for discoveryserveruniquename, server := range clientinstance.servers {
		server.lker.Lock()
		for _, peer := range server.allpeers {
			if peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] != peername {
				continue
			}
			msg := &RegMsg{}
			if e := json.Unmarshal(peer.regdata, msg); e != nil {
				//this is impossible
				fmt.Printf("[Discovery.client.WebSocketNotice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peer.peeruniquename, peer.regdata, discoveryserveruniquename)
				continue
			}
			if msg.WebSockPort == 0 {
				continue
			}
			addr := fmt.Sprintf("%s:%d", msg.WebSockIp, msg.WebSockPort)
			if _, ok := result[addr]; !ok {
				result[addr] = make(map[string]struct{}, 5)
			}
			result[addr][discoveryserveruniquename] = struct{}{}
		}
		server.lker.Unlock()
	}
	clientinstance.slker.RUnlock()
	return result, ch, nil
}

func (c *client) updateserver(cc *stream.TcpConfig, url string) {
	tker := time.NewTicker(time.Second)
	first := true
	for {
		if !first {
			<-tker.C
		}
		first = false
		//get server addrs
		resp, e := c.httpclient.Get(url)
		if e != nil {
			fmt.Printf("[Discovery.client.updateserver]get discovery server addr error:%s\n", e)
			continue
		}
		data, e := ioutil.ReadAll(resp.Body)
		if e != nil {
			fmt.Printf("[Discovery.client.updateserver]read response data error:%s\n", e)
			continue
		}
		serveraddrs := make([]string, 0)
		if e := json.Unmarshal(data, &serveraddrs); e != nil {
			fmt.Printf("[Discovery.client.updateserver]response data:%s format error:%s\n", data, e)
			continue
		}
		c.slker.Lock()
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
					lker:     &sync.Mutex{},
					peer:     nil,
					uniqueid: 0,
					htree:    hashtree.New(10, 3),
					allpeers: make(map[string]*peerinfo, 5),
				}
				go c.instance.StartTcpClient(cc, saddr[findex+1:], c.verifydata)
			}
		}
		c.slker.Unlock()
	}
}
func (c *client) verifyfunc(ctx context.Context, discoveryserveruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, discoveryserveruniquename string, uniqueid uint64) {
	c.slker.RLock()
	server, ok := c.servers[discoveryserveruniquename]
	c.slker.RUnlock()
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
func (c *client) userfunc(p *stream.Peer, discoveryserveruniquename string, uniqueid uint64, data []byte) {
	tempserver, e := p.GetData(uniqueid)
	if e != nil {
		//server closed
		return
	}
	server := (*servernode)(tempserver)
	server.lker.Lock()
	defer server.lker.Unlock()
	switch data[0] {
	case mSGONLINE:
		onlinepeer, regmsg, newhash, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]online peer:%s message:%s broken from discovery server:%s\n", onlinepeer, data, discoveryserveruniquename)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(onlinepeer, uint64(server.htree.GetLeavesNum())))
		templeafdata, _ := server.htree.GetLeafValue(leafindex)
		if templeafdata == nil {
			leafdata := make([]*peerinfo, 0, 5)
			temppeer := &peerinfo{
				peeruniquename: onlinepeer,
				regdata:        regmsg,
			}
			leafdata = append(leafdata, temppeer)
			server.allpeers[onlinepeer] = temppeer
			server.htree.SetSingleLeafHash(leafindex, str2byte(onlinepeer[:strings.Index(onlinepeer, ":")]+byte2str(regmsg)))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		} else {
			leafdata := *(*[]*peerinfo)(templeafdata)
			temppeer := &peerinfo{
				peeruniquename: onlinepeer,
				regdata:        regmsg,
			}
			leafdata = append(leafdata, temppeer)
			server.allpeers[onlinepeer] = temppeer
			sort.Slice(leafdata, func(i, j int) bool {
				return leafdata[i].peeruniquename < leafdata[j].peeruniquename
			})
			all := make([]string, len(leafdata))
			for i, peer := range leafdata {
				all[i] = peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] + byte2str(peer.regdata)
			}
			server.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]data from discovery server:%s conflict\n", discoveryserveruniquename)
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.nlker.RLock()
			c.notice(onlinepeer, regmsg, true, discoveryserveruniquename)
			c.nlker.RUnlock()
		}
	case mSGOFFLINE:
		offlinepeer, newhash, e := getOfflineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]offline peer:%s message:%s broken from discovery server:%s\n", offlinepeer, data, discoveryserveruniquename)
			p.Close(uniqueid)
			return
		}
		delete(server.allpeers, offlinepeer)
		leafindex := int(bkdrhash(offlinepeer, uint64(server.htree.GetLeavesNum())))
		templeafdata, _ := server.htree.GetLeafValue(leafindex)
		leafdata := *(*[]*peerinfo)(templeafdata)
		var regmsg []byte
		for i, peer := range leafdata {
			if peer.peeruniquename == offlinepeer {
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
				all[i] = peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] + byte2str(peer.regdata)
			}
			server.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
			server.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]data from discovery server:%s conflict\n", discoveryserveruniquename)
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.nlker.RLock()
			c.notice(offlinepeer, regmsg, false, discoveryserveruniquename)
			c.nlker.RUnlock()
		}
	case mSGPUSH:
		all, e := getPushMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]push message:%d broken from discovery server:%s\n", data, discoveryserveruniquename)
			p.Close(uniqueid)
			return
		}
		server.status = 2
		deleted := make(map[int]map[string][]byte, 5)
		added := make(map[int]map[string][]byte, 5)
		replace := make(map[int]map[string][]byte)
		updateleaveshash := make(map[int][]byte, 5)
		updateleavesvalue := make(map[int]unsafe.Pointer, 5)
		for _, oldpeer := range server.allpeers {
			if newmsg, ok := all[oldpeer.peeruniquename]; !ok {
				//delete
				leafindex := int(bkdrhash(oldpeer.peeruniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := deleted[leafindex]; !ok {
					deleted[leafindex] = make(map[string][]byte, 5)
				}
				deleted[leafindex][oldpeer.peeruniquename] = oldpeer.regdata
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			} else if !bytes.Equal(oldpeer.regdata, newmsg) {
				//replace
				leafindex := int(bkdrhash(oldpeer.peeruniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := replace[leafindex]; !ok {
					replace[leafindex] = make(map[string][]byte, 5)
				}
				replace[leafindex][oldpeer.peeruniquename] = newmsg
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			}
		}
		for newpeer, newmsg := range all {
			if oldpeer, ok := server.allpeers[newpeer]; !ok {
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
				fmt.Printf("[Discovery.client.userfunc.impossible]peer:%s regmsg conflict", oldpeer.peeruniquename)
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
			var leafdata []*peerinfo
			if templeafdata == nil {
				leafdata = make([]*peerinfo, 0, 5)
			} else {
				leafdata = *(*[]*peerinfo)(templeafdata)
			}
			//add
			for newpeer, newmsg := range added[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.peeruniquename == newpeer {
						//this is impossible
						find = true
						oldpeer.regdata = newmsg
						server.allpeers[newpeer] = oldpeer
						c.nlker.RLock()
						c.notice(newpeer, oldpeer.regdata, false, discoveryserveruniquename)
						c.notice(newpeer, newmsg, true, discoveryserveruniquename)
						c.nlker.RUnlock()
						break
					}
				}
				if !find {
					newnode := &peerinfo{peeruniquename: newpeer, regdata: newmsg}
					leafdata = append(leafdata, newnode)
					server.allpeers[newpeer] = newnode
					c.nlker.RLock()
					c.notice(newpeer, newmsg, true, discoveryserveruniquename)
					c.nlker.RUnlock()
				}
			}
			//replace
			for newpeer, newmsg := range replace[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.peeruniquename == newpeer {
						find = true
						oldpeer.regdata = newmsg
						server.allpeers[newpeer] = oldpeer
						c.nlker.RLock()
						c.notice(newpeer, oldpeer.regdata, false, discoveryserveruniquename)
						c.notice(newpeer, newmsg, true, discoveryserveruniquename)
						c.nlker.RUnlock()
						break
					}
				}
				if !find {
					//this is impossible
					server.allpeers[newpeer].regdata = newmsg
					leafdata = append(leafdata, server.allpeers[newpeer])
					c.nlker.RLock()
					c.notice(newpeer, newmsg, true, discoveryserveruniquename)
					c.nlker.RUnlock()
				}
			}
			//deleted
			pos := len(leafdata) - 1
			for oldpeer, oldmsg := range deleted[leafindex] {
				delete(server.allpeers, oldpeer)
				for i, v := range leafdata {
					if v.peeruniquename == oldpeer {
						if i != pos {
							leafdata[i], leafdata[pos] = leafdata[pos], leafdata[i]
						}
						pos--
						break
					}
				}
				c.nlker.RLock()
				c.notice(oldpeer, oldmsg, false, discoveryserveruniquename)
				c.nlker.RUnlock()
			}
			leafdata = leafdata[:pos+1]
			if len(leafdata) == 0 {
				updateleaveshash[leafindex] = nil
				updateleavesvalue[leafindex] = nil
			} else {
				sort.Slice(leafdata, func(i, j int) bool {
					return leafdata[i].peeruniquename < leafdata[j].peeruniquename
				})
				all := make([]string, len(leafdata))
				for i, peer := range leafdata {
					all[i] = peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] + byte2str(peer.regdata)
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
func (c *client) offlinefunc(p *stream.Peer, discoveryserveruniquename string) {
	c.slker.RLock()
	server, ok := c.servers[discoveryserveruniquename]
	c.slker.RUnlock()
	if !ok {
		return
	}
	server.lker.Lock()
	server.peer = nil
	server.uniqueid = 0
	server.htree.Reset()
	c.nlker.RLock()
	for _, peer := range server.allpeers {
		c.notice(peer.peeruniquename, peer.regdata, false, discoveryserveruniquename)
	}
	c.nlker.RUnlock()
	server.allpeers = make(map[string]*peerinfo)
	server.status = 0
	server.lker.Unlock()
}
func (c *client) notice(peeruniquename string, regmsg []byte, status bool, servername string) {
	msg := &RegMsg{}
	e := json.Unmarshal(regmsg, msg)
	if e != nil {
		//this is impossible
		fmt.Printf("[Discovery.client.notice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peeruniquename, regmsg, servername)
		return
	}
	name := peeruniquename[:strings.Index(peeruniquename, ":")]
	if notice, ok := c.grpcnotices[name]; ok && msg.GrpcPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.GrpcIp, msg.GrpcPort),
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.httpnotices[name]; ok && msg.HttpPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.HttpIp, msg.HttpPort),
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.tcpnotices[name]; ok && msg.TcpPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.TcpIp, msg.TcpPort),
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.webnotices[name]; ok && msg.WebSockPort != 0 {
		notice <- &NoticeMsg{
			PeerAddr:        fmt.Sprintf("%s:%d", msg.WebSockIp, msg.WebSockPort),
			Status:          status,
			DiscoveryServer: servername,
		}
	}
}
