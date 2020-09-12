package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

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
	name     string
	uniqueid uint64
	addr     string
	lker     *sync.Mutex
	htree    *hashtree.Hashtree
}

var clientinstance *client

func StartDiscoveryClient(c *stream.InstanceConfig, cc *stream.TcpConfig, vdata []byte, url string) {
	clientinstance = &client{
		lker:       &sync.RWMutex{},
		servers:    make(map[string]*servernode, 10),
		verifydata: vdata,
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
	}
	c.Verifyfunc = clientinstance.verifyfunc
	c.Onlinefunc = clientinstance.onlinefunc
	c.Userdatafunc = clientinstance.userfunc
	c.Offlinefunc = clientinstance.offlinefunc
	clientinstance.instance = stream.NewInstance(c)
	clientinstance.updateserver(cc, url)
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
				if saddr == v.addr {
					find = true
					break
				}
			}
			if !find {
				delete(c.servers, k)
			}
		}
		//online new server or reconnect to offline server
		for _, saddr := range serveraddrs {
			find := false
			for _, v := range c.servers {
				if v.addr == saddr {
					if v.uniqueid != 0 {
						find = true
					}
					break
				}
			}
			if !find {
				go func(saddr string) {
					if peernameip := c.instance.StartTcpClient(cc, saddr, c.verifydata); peernameip != "" {
						c.lker.RLock()
						if v, ok := c.servers[peernameip]; ok {
							v.addr = saddr
						}
						c.lker.RUnlock()
					}
				}(saddr)
			}
		}
		c.lker.Unlock()
	}
}
func (c *client) verifyfunc(ctx context.Context, peernameip string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, c.verifydata) {
		return nil, false
	}
	return nil, true
}
func (c *client) onlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
	c.lker.Lock()
	v, ok := c.servers[peernameip]
	if !ok {
		c.servers[peernameip] = &servernode{
			peer:     p,
			name:     peernameip,
			uniqueid: uniqueid,
			htree:    hashtree.New(10, 2),
			lker:     &sync.Mutex{},
		}
		c.lker.Unlock()
		return
	}
	c.lker.Unlock()
	if v.uniqueid == 0 {
		v.uniqueid = uniqueid
		return
	}
	//this is impossible
	fmt.Printf("[Discovery.client.onlinefunc]reconnect to discovery server")
	p.Close(uniqueid)
	return
}
func (c *client) userfunc(p *stream.Peer, peernameip string, uniqueid uint64, data []byte) {
	if len(data) <= 1 {
		return
	}
	switch data[0] {
	case MSGONLINE:
		onlinepeer, newhash, e := getOnlineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]online message broken,error:%s", e)
			return
		}
	case MSGOFFLINE:
		offlinepeer, newhash, e := getOfflineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.client.userfunc]offline message broken,error:%s", e)
			return
		}
	case MSGPUSH:
		all := getPushMsg(data)
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
func (c *client) offlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
	c.lker.RLock()
	if v, ok := c.servers[peernameip]; !ok {
		c.lker.RUnlock()
		return
	} else {
		v.lker.Lock()
		c.lker.RUnlock()
		v.uniqueid = 0
		v.htree.Reset()
		v.lker.Unlock()
	}
}
