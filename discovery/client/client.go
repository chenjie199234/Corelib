package client

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

	"github.com/chenjie199234/Corelib/discovery/msg"
	"github.com/chenjie199234/Corelib/hashtree"
	"github.com/chenjie199234/Corelib/stream"
)

type client struct {
	lker       *sync.RWMutex
	servers    map[string]*servernode
	verifydata []byte
	instance   *stream.Instance
	httpclient *http.Client
}

type servernode struct {
	peer     *stream.Peer
	uniqueid uint64
	htree    *hashtree.Hashtree
}

type hashtreeleafdata struct {
	peers      map[string]*msg.RegMsg
	peersindex []string
}

var instance *client

func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, url string) {
	if instance != nil {
		return
	}
	instance = &client{
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*servernode, 10),
		verifydata: vdata,
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
	}
	c.Verifyfunc = instance.verifyfunc
	c.Onlinefunc = instance.onlinefunc
	c.Userdatafunc = instance.userfunc
	c.Offlinefunc = instance.offlinefunc
	instance.instance = stream.NewInstance(c)
	instance.updateserver(cc, url)
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
					htree:    hashtree.New(10, 2),
				}
				go c.instance.StartTcpClient(cc, saddr[findex+1:], c.verifydata)
			}
		}
		c.lker.Unlock()
	}
}
func (c *client) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	c.lker.RLock()
	v, ok := c.servers[peeruniquename]
	c.lker.RUnlock()
	if !ok {
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
	p.SendMessage(msg.MakePullMsg(), uniqueid)
	return
}
func (c *client) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	if len(data) <= 1 {
		return
	}
	c.lker.RLock()
	server, ok := c.servers[peeruniquename]
	c.lker.RUnlock()
	if !ok {
		//this server already offline,the offlinefunc will be called later
		return
	}
	if server.uniqueid != uniqueid {
		//this is impossible
		fmt.Printf("[Discovery.client.userfunc]online peer:%s has different uniqueid", peeruniquename)
		return
	}
	switch data[0] {
	case msg.MSGONLINE:
		onlinepeer, regmsg, newhash, e := msg.GetOnlineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]online message:%s broken,error:%s\n", data, e)
			return
		}
		leafindex := int(msg.Bkdrhash(onlinepeer, uint64(server.htree.GetLeavesNum())))
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := &hashtreeleafdata{
				peers:      make(map[string]*msg.RegMsg),
				peersindex: make([]string, 0, 5),
			}
			name := onlinepeer[:strings.Index(onlinepeer, ":")]
			leafdata.peers[onlinepeer] = regmsg
			leafdata.peersindex = append(leafdata.peersindex, onlinepeer)
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: msg.Str2byte(name + regmsg.GrpcAddr + regmsg.HttpAddr + regmsg.TcpAddr),
				Value:   unsafe.Pointer(leafdata),
			})
		} else {
			leafdata := (*hashtreeleafdata)(leaf.Value)
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
				peer := leafdata.peers[indexname]
				all[i] = indexname[:strings.Index(indexname, ":")] + peer.GrpcAddr + peer.HttpAddr + peer.TcpAddr
			}
			server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: msg.Str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(msg.MakePullMsg(), uniqueid)
		}
	case msg.MSGOFFLINE:
		offlinepeer, newhash, e := msg.GetOfflineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]offline message:%s broken,error:%s", data, e)
			return
		}
		leafindex := int(msg.Bkdrhash(offlinepeer, uint64(server.htree.GetLeavesNum())))
		leaf, _ := server.htree.GetLeaf(leafindex)
		if leaf == nil {
			return
		}
		leafdata := (*hashtreeleafdata)(leaf.Value)
		if _, ok := leafdata.peers[offlinepeer]; !ok {
			return
		}
		delete(leafdata.peers, offlinepeer)
		for i, peername := range leafdata.peersindex {
			if peername == offlinepeer {
				leafdata.peersindex = append(leafdata.peersindex[:i], leafdata.peersindex[i+1:]...)
				break
			}
		}
		all := make([]string, len(leafdata.peersindex))
		for i, indexname := range leafdata.peersindex {
			peer, _ := leafdata.peers[indexname]
			all[i] = indexname[:strings.Index(indexname, ":")] + peer.GrpcAddr + peer.HttpAddr + peer.TcpAddr
		}
		server.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: msg.Str2byte(strings.Join(all, "")),
			Value:   unsafe.Pointer(leafdata),
		})
		if !bytes.Equal(server.htree.GetRootHash(), newhash) {
			p.SendMessage(msg.MakePullMsg(), uniqueid)
		}
	case msg.MSGPUSH:
		all, e := msg.GetPushMsg(data)
		c.lker.RLock()
		server, ok := c.servers[peernameip]
		if !ok {
			c.lker.RUnlock()
			return
		}
		data := make([]*hashtree.LeafData, server.htree.GetLeavesNum())
		for _, peernameip := range all {
			where := int(bkdrhash(byte2str(peernameip), uint64(server.htree.GetLeavesNum())))
		}
		server.htree.Rebuild()
		c.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.client.userfunc]unknown message type")
		p.Close(uniqueid)
	}
}
func (c *client) offlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	c.lker.RLock()
	v, ok := c.servers[peeruniquename]
	c.lker.RUnlock()
	if !ok {
		return
	}
	v.peer = nil
	v.uniqueid = 0
	v.htree.Reset()
}
