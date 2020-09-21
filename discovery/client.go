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
}

type servernode struct {
	peer     *stream.Peer
	uniqueid uint64
	htree    *hashtree.Hashtree
	allpeers map[string]*peerinfo //key peeruniquename
}
type peerinfo struct {
	peeruniquename string
	regdata        []byte
}

var clientinstance *client

func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, regmsg *RegMsg, url string) {
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
	clientinstance.slker.RLock()
	regmsg, _ := json.Marshal(clientinstance.regmsg)
	for _, server := range clientinstance.servers {
		server.peer.SendMessage(makeOnlineMsg("", regmsg, nil), server.uniqueid)
	}
	clientinstance.slker.RUnlock()
}

//func GrpcNotice(peername string) (chan *NoticeMsg, error) {
//}
//func HttpNotice(peername string) (chan *NoticeMsg, error) {
//}
//func TcpNotice(peername string) (chan *NoticeMsg, error) {
//}
//func WebSocketNotice(peername string) (chan *NoticeMsg, error) {
//}
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
	v, ok := c.servers[discoveryserveruniquename]
	c.slker.RUnlock()
	if !ok {
		//this discovery server had already been unregistered
		return
	}
	if v.uniqueid != 0 || v.peer != nil {
		//this is impossible
		fmt.Printf("[Discovery.client.onlinefunc.impossible]duplicate connection from discovery server:%s", discoveryserveruniquename)
		p.Close(uniqueid)
		return
	}
	v.peer = p
	v.uniqueid = uniqueid
	//after online the first message is pull all registered peers
	p.SendMessage(makePullMsg(), uniqueid)
	//second message is reg self
	regmsg, _ := json.Marshal(c.regmsg)
	p.SendMessage(makeOnlineMsg("", regmsg, nil), uniqueid)
	return
}
func (c *client) userfunc(p *stream.Peer, discoveryserveruniquename string, uniqueid uint64, data []byte) {
	c.slker.RLock()
	server, ok := c.servers[discoveryserveruniquename]
	c.slker.RUnlock()
	if !ok {
		//this discovery server had already been unregistered,the offlinefunc will be called later
		return
	}
	if (server.peer != nil) && (server.uniqueid != uniqueid) {
		//this is impossible
		fmt.Printf("[Discovery.client.userfunc.impossible]online discovery server:%s has different uniqueid\n", discoveryserveruniquename)
		p.Close(uniqueid)
		return
	}
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
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := make([]*peerinfo, 0, 5)
			temppeer := &peerinfo{
				peeruniquename: onlinepeer,
				regdata:        regmsg,
			}
			leafdata = append(leafdata, temppeer)
			server.allpeers[onlinepeer] = temppeer
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(onlinepeer[:strings.Index(onlinepeer, ":")] + byte2str(regmsg)),
				Value:   unsafe.Pointer(&leafdata),
			})
		} else {
			leafdata := *(*[]*peerinfo)(leaf.Value)
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
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(&leafdata),
			})
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.notice(onlinepeer, regmsg, true, discoveryserveruniquename)
		}
	case mSGOFFLINE:
		offlinepeer, newhash, e := getOfflineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]offline peer:%s message:%s broken from discovery server:%s\n", offlinepeer, data, discoveryserveruniquename)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(offlinepeer, uint64(server.htree.GetLeavesNum())))
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			//this is impossible,but don't need to do anything
			fmt.Printf("[Discovery.client.userfunc.impossible]peer:%s hashtree data missing\n", offlinepeer)
			p.Close(uniqueid)
			return
		}
		leafdata := *(*[]*peerinfo)(leaf.Value)
		var regmsg []byte
		for i, peer := range leafdata {
			if peer.peeruniquename == offlinepeer {
				leafdata = append(leafdata[:i], leafdata[i+1:]...)
				regmsg = peer.regdata
				break
			}
		}
		if len(leafdata) == 0 {
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: nil,
				Value:   nil,
			})
		} else {
			all := make([]string, len(leafdata))
			for i, peer := range leafdata {
				all[i] = peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] + byte2str(peer.regdata)
			}
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(&leafdata),
			})
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(makePullMsg(), uniqueid)
		} else {
			c.notice(offlinepeer, regmsg, false, discoveryserveruniquename)
		}
	case mSGPUSH:
		all, e := getPushMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.client.userfunc.impossible]push message:%d broken from discovery server:%s\n", data, discoveryserveruniquename)
			p.Close(uniqueid)
			return
		}
		deleted := make(map[int]map[string][]byte, 5)
		added := make(map[int]map[string][]byte, 5)
		replace := make(map[int]map[string][]byte)
		updateleaves := make(map[int]*hashtree.LeafData, 5)
		for _, oldpeer := range server.allpeers {
			if newmsg, ok := all[oldpeer.peeruniquename]; !ok {
				//delete
				leafindex := int(bkdrhash(oldpeer.peeruniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := deleted[leafindex]; !ok {
					deleted[leafindex] = make(map[string][]byte, 5)
				}
				deleted[leafindex][oldpeer.peeruniquename] = oldpeer.regdata
				updateleaves[leafindex] = nil
			} else if !bytes.Equal(oldpeer.regdata, newmsg) {
				//this is impossible
				fmt.Printf("[Discovery.client.userfunc.impossible]peer:%s regmsg conflict", oldpeer.peeruniquename)
				//replace
				leafindex := int(bkdrhash(oldpeer.peeruniquename, uint64(server.htree.GetLeavesNum())))
				if _, ok := replace[leafindex]; !ok {
					replace[leafindex] = make(map[string][]byte, 5)
				}
				replace[leafindex][oldpeer.peeruniquename] = newmsg
				updateleaves[leafindex] = nil
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
				updateleaves[leafindex] = nil
			} else if !bytes.Equal(oldpeer.regdata, newmsg) {
				//this is impossible
				fmt.Printf("[Discovery.client.userfunc.impossible]peer:%s regmsg conflict", oldpeer.peeruniquename)
				//replace add
				leafindex := int(bkdrhash(newpeer, uint64(server.htree.GetLeavesNum())))
				if _, ok := replace[leafindex]; !ok {
					replace[leafindex] = make(map[string][]byte, 5)
				}
				replace[leafindex][newpeer] = newmsg
				updateleaves[leafindex] = nil
			}
		}
		for leafindex := range updateleaves {
			leaf, _ := server.htree.GetLeaf(leafindex)
			var leafdata []*peerinfo
			if leaf == nil {
				leafdata = make([]*peerinfo, 0, 5)
			} else {
				leafdata = *(*[]*peerinfo)(leaf.Value)
			}
			//add
			for newpeer, newmsg := range added[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.peeruniquename == newpeer {
						//this is impossible
						find = true
						c.notice(newpeer, oldpeer.regdata, false, discoveryserveruniquename)
						oldpeer.regdata = newmsg
						server.allpeers[newpeer] = oldpeer
						c.notice(newpeer, newmsg, true, discoveryserveruniquename)
						break
					}
				}
				if !find {
					newnode := &peerinfo{peeruniquename: newpeer, regdata: newmsg}
					leafdata = append(leafdata, newnode)
					server.allpeers[newpeer] = newnode
					c.notice(newpeer, newmsg, true, discoveryserveruniquename)
				}
			}
			//replace
			for newpeer, newmsg := range replace[leafindex] {
				find := false
				for _, oldpeer := range leafdata {
					if oldpeer.peeruniquename == newpeer {
						c.notice(newpeer, oldpeer.regdata, false, discoveryserveruniquename)
						find = true
						oldpeer.regdata = newmsg
						c.notice(newpeer, newmsg, true, discoveryserveruniquename)
						break
					}
				}
				if !find {
					//this is impossible
					leafdata = append(leafdata, &peerinfo{peeruniquename: newpeer, regdata: newmsg})
					c.notice(newpeer, newmsg, true, discoveryserveruniquename)
				}
			}
			//deleted
			pos := len(leafdata) - 1
			for oldpeer, oldmsg := range deleted[leafindex] {
				c.notice(oldpeer, oldmsg, false, discoveryserveruniquename)
				delete(server.allpeers, oldpeer)
				for i, v := range leafdata {
					if v.peeruniquename == oldpeer {
						if i != pos {
							leafdata[i], leafdata[pos] = leafdata[pos], leafdata[i]
						}
						pos--
					}
				}
			}
			leafdata = leafdata[:pos+1]
			if len(leafdata) == 0 {
				updateleaves[leafindex] = &hashtree.LeafData{
					Hashstr: nil,
					Value:   nil,
				}
			} else {
				sort.Slice(leafdata, func(i, j int) bool {
					return leafdata[i].peeruniquename < leafdata[j].peeruniquename
				})
				all := make([]string, len(leafdata))
				for i, peer := range leafdata {
					all[i] = peer.peeruniquename[:strings.Index(peer.peeruniquename, ":")] + byte2str(peer.regdata)
				}
				updateleaves[leafindex] = &hashtree.LeafData{
					Hashstr: str2byte(strings.Join(all, "")),
					Value:   unsafe.Pointer(&leafdata),
				}
			}
		}
		server.htree.SetMultiLeaves(updateleaves)
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
	server.peer = nil
	server.uniqueid = 0
	leaves := server.htree.GetAllLeaf()
	for _, leaf := range leaves {
		leafdata := *(*[]*peerinfo)(leaf.Value)
		for _, peer := range leafdata {
			c.notice(peer.peeruniquename, peer.regdata, false, discoveryserveruniquename)
		}
	}
	server.htree.Reset()
}
func (c *client) notice(peeruniquename string, regmsg []byte, status bool, servername string) {
	msg := &RegMsg{}
	e := json.Unmarshal(regmsg, msg)
	if e != nil {
		fmt.Printf("[Discovery.client.notice.impossible]peer:%s regmsg:%s broken from discovery server:%s\n", peeruniquename, regmsg, servername)
		return
	}
	name := peeruniquename[:strings.Index(peeruniquename, ":")]
	if notice, ok := c.grpcnotices[name]; ok && msg.GrpcAddr != "" {
		notice <- &NoticeMsg{
			PeerAddr:        msg.GrpcAddr,
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.httpnotices[name]; ok && msg.HttpAddr != "" {
		notice <- &NoticeMsg{
			PeerAddr:        msg.HttpAddr,
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.tcpnotices[name]; ok && msg.TcpAddr != "" {
		notice <- &NoticeMsg{
			PeerAddr:        msg.TcpAddr,
			Status:          status,
			DiscoveryServer: servername,
		}
	}
	if notice, ok := c.webnotices[name]; ok && msg.WebSocketAddr != "" {
		notice <- &NoticeMsg{
			PeerAddr:        msg.WebSocketAddr,
			Status:          status,
			DiscoveryServer: servername,
		}
	}
}
