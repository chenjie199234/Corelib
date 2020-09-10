package discovery

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chenjie199234/Corelib/buckettree"
	"github.com/chenjie199234/Corelib/stream"
)

type server struct {
	lker       *sync.RWMutex
	hashtree   *buckettree.BucketTree
	clients    [][]*clientnode
	verifydata []byte
	nodepool   *sync.Pool
	instance   *stream.Instance
}

type clientnode struct {
	peer     *stream.Peer
	name     string
	uniqueid uint64
}

var serverinstance *server

func (s *server) getnode(peer *stream.Peer, name string, uniqueid uint64) *clientnode {
	result := s.nodepool.Get().(*clientnode)
	result.name = name
	result.peer = peer
	result.uniqueid = uniqueid
	return result
}
func (s *server) putnode(n *clientnode) {
	n.peer = nil
	n.name = ""
	n.uniqueid = 0
	s.nodepool.Put(n)
}

func StartDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	serverinstance = &server{
		lker:     &sync.RWMutex{},
		hashtree: buckettree.New(10, 2),
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &clientnode{}
			},
		},
		verifydata: vdata,
	}
	//hash
	serverinstance.clients = make([][]*clientnode, serverinstance.hashtree.GetBucketNum())
	for i := range serverinstance.clients {
		serverinstance.clients[i] = make([]*clientnode, 0)
	}
	//instance
	c.Verifyfunc = serverinstance.verifyfunc
	c.Onlinefunc = serverinstance.onlinefunc
	c.Userdatafunc = serverinstance.userfunc
	c.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance = stream.NewInstance(c)
	serverinstance.instance.StartTcpServer(cc, listenaddr)
}
func (s *server) verifyfunc(ctx context.Context, peername string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}
func (s *server) onlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
	where := bkdrhash(peernameip, s.hashtree.GetBucketNum())
	s.lker.Lock()
	s.clients[where] = append(s.clients[where], s.getnode(p, peernameip, uniqueid))
	sort.Slice(s.clients[where], func(i, j int) bool {
		return s.clients[where][i].name < s.clients[where][j].name
	})
	all := make([]string, len(s.clients[where]))
	for i, v := range s.clients[where] {
		all[i] = v.name
	}
	s.hashtree.UpdateSingle(where, str2byte(strings.Join(all, "")))
	//notice all other peer
	onlinedata := makeOnlineMsg(peernameip, s.hashtree.GetRootHash())
	for _, v := range s.clients {
		for _, vv := range v {
			if vv.name != peernameip {
				vv.peer.SendMessage(onlinedata, vv.uniqueid)
			}
		}
	}
	//push all data to this new peer
	count := 0
	for _, v := range s.clients {
		count += len(v)
	}
	result := make([]string, count)
	count = 0
	for _, v := range s.clients {
		for _, vv := range v {
			result[count] = vv.name
			count++
		}
	}
	p.SendMessage(makePushMsg(result), uniqueid)
	s.lker.Unlock()
}
func (s *server) userfunc(ctx context.Context, p *stream.Peer, peernameip string, uniqueid uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case MSGPULL:
		s.lker.RLock()
		count := 0
		for _, v := range s.clients {
			count += len(v)
		}
		result := make([]string, count)
		count = 0
		for _, v := range s.clients {
			for _, vv := range v {
				result[count] = vv.name
				count++
			}
		}
		p.SendMessage(makePushMsg(result), uniqueid)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type")
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
	where := bkdrhash(peernameip, s.hashtree.GetBucketNum())
	s.lker.Lock()
	for i, v := range s.clients[where] {
		if v.name == peernameip && v.uniqueid == uniqueid {
			s.clients[where] = append(s.clients[where][:i], s.clients[where][i+1:]...)
			s.putnode(v)
			break
		}
	}
	all := make([]string, len(s.clients[where]))
	for i, v := range s.clients[where] {
		all[i] = v.name
	}
	s.hashtree.UpdateSingle(where, str2byte(strings.Join(all, "")))
	data := makeOfflineMsg(peernameip, s.hashtree.GetRootHash())
	for _, v := range s.clients {
		for _, vv := range v {
			vv.peer.SendMessage(data, vv.uniqueid)
		}
	}
	s.lker.Unlock()
}
