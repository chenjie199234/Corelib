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
	nodes      [][]*node
	nodepool   *sync.Pool
	verifydata []byte
	instance   *stream.Instance
}

var serverinstance *server

func (s *server) getnode(peer *stream.Peer, name string, uniqueid uint64) *node {
	result := s.nodepool.Get().(*node)
	result.n = name
	result.p = peer
	result.u = uniqueid
	return result
}
func (s *server) putnode(n *node) {
	n.p = nil
	n.n = ""
	n.u = 0
	s.nodepool.Put(n)
}

func StartDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	serverinstance = &server{
		lker:     &sync.RWMutex{},
		hashtree: buckettree.New(10, 2),
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &node{}
			},
		},
		verifydata: vdata,
	}
	//hash
	serverinstance.nodes = make([][]*node, serverinstance.hashtree.GetBucketNum())
	for i := range serverinstance.nodes {
		serverinstance.nodes[i] = make([]*node, 0)
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
	s.nodes[where] = append(s.nodes[where], s.getnode(p, peernameip, uniqueid))
	sort.Slice(s.nodes[where], func(i, j int) bool {
		return s.nodes[where][i].n < s.nodes[where][j].n
	})
	all := make([]string, len(s.nodes[where]))
	for i, v := range s.nodes[where] {
		all[i] = v.n
	}
	s.hashtree.UpdateSingle(where, str2byte(strings.Join(all, "")))
	data := makeOnlineMsg(peernameip, s.hashtree.GetRootHash())
	for _, v := range s.nodes {
		for _, vv := range v {
			vv.p.SendMessage(data, vv.u)
		}
	}
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
		for _, v := range s.nodes {
			count += len(v)
		}
		result := make([]string, count)
		count = 0
		for _, v := range s.nodes {
			for _, vv := range v {
				result[count] = vv.n
				count++
			}
		}
		s.lker.RUnlock()
		data := makePushMsg(result)
		p.SendMessage(data, uniqueid)
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type")
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peernameip string, uniqueid uint64) {
	where := bkdrhash(peernameip, s.hashtree.GetBucketNum())
	s.lker.Lock()
	for i, v := range s.nodes[where] {
		if v.n == peernameip {
			s.nodes[where] = append(s.nodes[where][:i], s.nodes[where][i+1:]...)
			s.putnode(v)
			break
		}
	}
	all := make([]string, len(s.nodes[where]))
	for i, v := range s.nodes[where] {
		all[i] = v.n
	}
	s.hashtree.UpdateSingle(where, str2byte(strings.Join(all, "")))
	data := makeOfflineMsg(peernameip, s.hashtree.GetRootHash())
	for _, v := range s.nodes {
		for _, vv := range v {
			vv.p.SendMessage(data, vv.u)
		}
	}
	s.lker.Unlock()
}
