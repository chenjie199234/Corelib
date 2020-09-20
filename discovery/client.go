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
	ERREXISTS  = fmt.Errorf("already register this peer's notice")
)

type client struct {
	slker       *sync.RWMutex
	servers     map[string]*servernode
	verifydata  []byte
	instance    *stream.Instance
	httpclient  *http.Client
	regmsg      *RegMsg
	nlker       *sync.Mutex
	grpcnotices map[string]chan *NoticeMsg
	httpnotices map[string]chan *NoticeMsg
	tcpnotices  map[string]chan *NoticeMsg
	webnotices  map[string]chan *NoticeMsg
	status      bool //true started,false not started,notices can't be registered after started
}

type servernode struct {
	peer     *stream.Peer
	uniqueid uint64
	htree    *hashtree.Hashtree
}

type clienthashtreeleafdata struct {
	//key peeruniquename,value reg data
	peers      map[string][]byte
	peersindex []string
}

var clientinstance *client

func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, regmsg *RegMsg, url string) {
	d, _ := json.Marshal(regmsg)
	if bytes.Contains(d, []byte{sPLIT}) {
		panic("[Discovery.client.StartDiscoveryClient]regmsg contains illegal character '|'")
	}
	clientinstance.verifydata = vdata
	clientinstance.regmsg = regmsg

	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)

	clientinstance.nlker.Lock()
	clientinstance.status = true
	clientinstance.nlker.Unlock()
	go clientinstance.updateserver(cc, url)
}
func GrpcNotice(peername string) (chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if clientinstance.status {
		clientinstance.nlker.Unlock()
		return nil, ERRSTARTED
	}
	if _, ok := clientinstance.grpcnotices[peername]; ok {
		return nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 10)
	clientinstance.grpcnotices[peername] = ch
	clientinstance.nlker.Unlock()
	return ch, nil
}
func HttpNotice(peername string) (chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if clientinstance.status {
		clientinstance.nlker.Unlock()
		return nil, ERRSTARTED
	}
	if _, ok := clientinstance.httpnotices[peername]; ok {
		return nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 10)
	clientinstance.httpnotices[peername] = ch
	clientinstance.nlker.Unlock()
	return ch, nil
}
func TcpNotice(peername string) (chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if clientinstance.status {
		clientinstance.nlker.Unlock()
		return nil, ERRSTARTED
	}
	if _, ok := clientinstance.tcpnotices[peername]; ok {
		return nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 10)
	clientinstance.tcpnotices[peername] = ch
	clientinstance.nlker.Unlock()
	return ch, nil
}
func WebSocketNotice(peername string) (chan *NoticeMsg, error) {
	clientinstance.nlker.Lock()
	if clientinstance.status {
		clientinstance.nlker.Unlock()
		return nil, ERRSTARTED
	}
	if _, ok := clientinstance.webnotices[peername]; ok {
		return nil, ERREXISTS
	}
	ch := make(chan *NoticeMsg, 10)
	clientinstance.webnotices[peername] = ch
	clientinstance.nlker.Unlock()
	return ch, nil
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
				}
				go c.instance.StartTcpClient(cc, saddr[findex+1:], c.verifydata)
			}
		}
		c.slker.Unlock()
	}
}
func (c *client) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	c.slker.RLock()
	v, ok := c.servers[peeruniquename]
	c.slker.RUnlock()
	if !ok {
		//this discovery server had already been unregistered
		return
	}
	if v.uniqueid != 0 || v.peer != nil {
		//this is impossible
		fmt.Printf("[Discovery.client.onlinefunc]reconnect to discovery server:%s", peeruniquename)
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
func (c *client) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	c.slker.RLock()
	server, ok := c.servers[peeruniquename]
	c.slker.RUnlock()
	if !ok {
		//this discovery server had already been unregistered,the offlinefunc will be called later
		return
	}
	if (server.peer != nil) && (server.uniqueid != uniqueid) {
		//this is impossible
		fmt.Printf("[Discovery.client.userfunc]online peer:%s has different uniqueid\n", peeruniquename)
		p.Close(uniqueid)
		return
	}
	switch data[0] {
	case mSGONLINE:
		onlinepeer, regmsg, newhash, e := getOnlineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]online message:%s broken\n", data)
			p.Close(uniqueid)
			return
		}
		findex := strings.Index(onlinepeer, ":")
		if findex == -1 || len(onlinepeer) == findex+1 {
			fmt.Printf("[Discovery.client.userfunc]online peer addr:%s format error\n", onlinepeer)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(onlinepeer, uint64(server.htree.GetLeavesNum())))
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := &clienthashtreeleafdata{
				peers:      map[string][]byte{onlinepeer: regmsg},
				peersindex: []string{onlinepeer},
			}
			name := onlinepeer[:findex]
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(name + byte2str(regmsg)),
				Value:   unsafe.Pointer(leafdata),
			})
		} else {
			leafdata := (*clienthashtreeleafdata)(leaf.Value)
			if _, ok := leafdata.peers[onlinepeer]; ok {
				//this is impossible
				fmt.Printf("[Discovery.client.userfunc]duplicate peer:%s reg online\n", onlinepeer)
				leafdata.peers[onlinepeer] = regmsg
			} else {
				leafdata.peers[onlinepeer] = regmsg
				leafdata.peersindex = append(leafdata.peersindex, onlinepeer)
			}
			sort.Strings(leafdata.peersindex)
			all := make([]string, len(leafdata.peersindex))
			for i, indexname := range leafdata.peersindex {
				all[i] = indexname[:strings.Index(indexname, ":")] + byte2str(leafdata.peers[indexname])
			}
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(makePullMsg(), uniqueid)
		}
	case mSGOFFLINE:
		offlinepeer, newhash, e := getOfflineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]offline message:%s broken\n", data)
			p.Close(uniqueid)
			return
		}
		findex := strings.Index(offlinepeer, ":")
		if findex == -1 || len(offlinepeer) == findex+1 {
			fmt.Printf("[Discovery.client.userfunc]offline peer addr:%s format error\n", offlinepeer)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(offlinepeer, uint64(server.htree.GetLeavesNum())))
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			//this is impossible,but don't need to do anything
			return
		}
		leafdata := (*clienthashtreeleafdata)(leaf.Value)
		if _, ok := leafdata.peers[offlinepeer]; !ok {
			//this is impossible,but don't need to do anything
			return
		}
		delete(leafdata.peers, offlinepeer)
		for i, peername := range leafdata.peersindex {
			if peername == offlinepeer {
				leafdata.peersindex = append(leafdata.peersindex[:i], leafdata.peersindex[i+1:]...)
				break
			}
		}
		if len(leafdata.peers) == 0 {
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: nil,
				Value:   nil,
			})
		} else {
			all := make([]string, len(leafdata.peersindex))
			for i, indexname := range leafdata.peersindex {
				all[i] = indexname[:strings.Index(indexname, ":")] + byte2str(leafdata.peers[indexname])
			}
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(makePullMsg(), uniqueid)
		}
	case mSGPUSH:
		all, e := getPushMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]push message:%d broken\n", data)
			p.Close(uniqueid)
			return
		}
		updatedatas := make(map[int]map[string][]byte)
		for onlinepeer, regmsg := range all {
			leafindex := int(bkdrhash(onlinepeer, uint64(server.htree.GetLeavesNum())))
			if _, ok := updatedatas[leafindex]; !ok {
				updatedatas[leafindex] = make(map[string][]byte, 10)
			}
			updatedatas[leafindex][onlinepeer] = regmsg
		}
		allleaves := make([]*hashtree.LeafData, server.htree.GetLeavesNum())
		for i := range allleaves {
			allleaves[i] = &hashtree.LeafData{
				Hashstr: nil,
				Value:   nil,
			}
			leaf, _ := server.htree.GetLeaf(i)
			datas := updatedatas[i]
			var leafdata *clienthashtreeleafdata
			status := 0
			if leaf == nil {
				//origin doesn't exist
				if len(datas) > 0 {
					//new peers online
					leafdata = &clienthashtreeleafdata{
						peers:      make(map[string][]byte, int(float64(len(datas))*1.3)),
						peersindex: make([]string, 0, int(float64(len(datas))*1.3)),
					}
					status = 1
				}
			} else {
				leafdata = (*clienthashtreeleafdata)(leaf.Value)
				if len(leafdata.peers) == 0 {
					//origin doesn't exist
					if len(datas) > 0 {
						//new peers online
						status = 1
					} else {
						leafdata = nil
					}
				} else {
					//origin exist
					if len(datas) == 0 {
						//origin peers offline
						status = 2
					} else {
						status = 3
					}
				}
			}
			switch status {
			case 1:
				//new peers online
				for onlinepeer, regmsg := range datas {
					leafdata.peers[onlinepeer] = regmsg
					leafdata.peersindex = append(leafdata.peersindex, onlinepeer)
				}
			case 2:
				//origin peers offline
				leafdata = nil
			case 3:
				//origin peers offline
				index := len(leafdata.peersindex) - 1
				for originpeer, originregmsg := range leafdata.peers {
					find := false
					for newpeer, newregmsg := range datas {
						if originpeer == newpeer {
							find = true
							if !bytes.Equal(originregmsg, newregmsg) {
								//this is impossible
								leafdata.peers[originpeer] = newregmsg
								fmt.Printf("[Discovery.client.userfunc]peer:%s reg message conflict,new reg msg replace the old\n", originpeer)
							}
							break
						}
					}
					if !find {
						//offline
						delete(leafdata.peers, originpeer)
						for i, v := range leafdata.peersindex {
							if v == originpeer {
								leafdata.peersindex[i], leafdata.peersindex[index] = leafdata.peersindex[index], leafdata.peersindex[i]
								index--
								break
							}
						}
					}
				}
				if index != len(leafdata.peersindex)-1 {
					leafdata.peersindex = leafdata.peersindex[:index+1]
				}
				//new peers online
				for newpeer, newregmsg := range datas {
					find := false
					for originpeer, originregmsg := range leafdata.peers {
						if newpeer == originpeer {
							find = true
							if !bytes.Equal(newregmsg, originregmsg) {
								//this is impossible
								leafdata.peers[originpeer] = newregmsg
								fmt.Printf("[Discovery.client.userfunc]peer:%s reg message conflict,new reg msg replace the old\n", originpeer)
							}
							break
						}
					}
					if !find {
						//online
						leafdata.peers[newpeer] = newregmsg
						leafdata.peersindex = append(leafdata.peersindex, newpeer)
					}
				}
			}
			if leafdata != nil {
				sort.Strings(leafdata.peersindex)
				all := make([]string, len(leafdata.peersindex))
				for i, indexname := range leafdata.peersindex {
					all[i] = indexname[:strings.Index(indexname, ":")] + byte2str(leafdata.peers[indexname])
				}
				allleaves[i].Hashstr = str2byte(strings.Join(all, ""))
				allleaves[i].Value = unsafe.Pointer(leafdata)
			}
		}
		server.htree.Rebuild(allleaves)
	default:
		fmt.Printf("[Discovery.client.userfunc]unknown message type")
		p.Close(uniqueid)
	}
}
func (c *client) offlinefunc(p *stream.Peer, peeruniquename string) {
	c.slker.RLock()
	server, ok := c.servers[peeruniquename]
	c.slker.RUnlock()
	if !ok {
		return
	}
	server.peer = nil
	server.uniqueid = 0
	//leaves := server.htree.GetAllLeaf()
	//for _, leaf := range leaves {

	//}
	server.htree.Reset()
}
