package discovery

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/hashtree"
	"github.com/chenjie199234/Corelib/stream"
)

type server struct {
	lker       *sync.RWMutex
	htree      *hashtree.Hashtree
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
		lker:  &sync.RWMutex{},
		htree: hashtree.New(10, 2),
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &clientnode{}
			},
		},
		verifydata: vdata,
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
	leafindex := int(bkdrhash(peernameip, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	leaf, _ := s.htree.GetLeaf(leafindex)
	if leaf != nil {
		clients := make([]*clientnode, 1)
		clients[0] = s.getnode(p, peernameip, uniqueid)
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: str2byte(peernameip),
			Value:   unsafe.Pointer(&clients),
		})
	} else {
		clients := *((*[]*clientnode)(leaf.Value))
		clients = append(clients, s.getnode(p, peernameip, uniqueid))
		sort.Slice(clients, func(i, j int) bool {
			return clients[i].name < clients[j].name
		})
		all := make([]string, len(clients))
		for i, client := range clients {
			all[i] = client.name
		}
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: str2byte(strings.Join(all, "")),
			Value:   unsafe.Pointer(&clients),
		})
	}
	leaves := s.htree.GetAllLeaf()
	count := 0
	//notice all other peer
	onlinedata := makeOnlineMsg(peernameip, s.htree.GetRootHash())
	for _, leaf := range leaves {
		clients := *(*[]*clientnode)(leaf.Value)
		count += len(clients)
		for _, client := range clients {
			if client.name != peernameip {
				client.peer.SendMessage(onlinedata, client.uniqueid)
			}
		}
	}
	//push all data to this new peer
	result := make([]string, count)
	count = 0
	for _, leaf := range leaves {
		for _, client := range *(*[]*clientnode)(leaf.Value) {
			result[count] = client.name
			count++
		}
	}
	p.SendMessage(makePushMsg(result), uniqueid)
	s.lker.Unlock()
}
func (s *server) userfunc(p *stream.Peer, peernameip string, uniqueid uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case MSGPULL:
		s.lker.RLock()
		leaves := s.htree.GetAllLeaf()
		count := 0
		for _, leaf := range leaves {
			count += len(*(*[]*clientnode)(leaf.Value))
		}
		result := make([]string, count)
		count = 0
		for _, leaf := range leaves {
			for _, client := range *(*[]*clientnode)(leaf.Value) {
				result[count] = client.name
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
	where := int(bkdrhash(peernameip, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	leaf, _ := s.htree.GetLeaf(where)
	clients := *(*[]*clientnode)(leaf.Value)
	for i, client := range clients {
		if client.name == peernameip && client.uniqueid == uniqueid {
			clients = append(clients[:i], clients[i+1:]...)
			s.putnode(client)
			break
		}
	}
	all := make([]string, len(clients))
	for i, client := range clients {
		all[i] = client.name
	}
	s.htree.SetSingleLeaf(where, &hashtree.LeafData{
		Hashstr: str2byte(strings.Join(all, "")),
		Value:   unsafe.Pointer(&clients),
	})
	//notice all other peer
	data := makeOfflineMsg(peernameip, s.htree.GetRootHash())
	leaves := s.htree.GetAllLeaf()
	for _, leaf := range leaves {
		for _, client := range *(*[]*clientnode)(leaf.Value) {
			client.peer.SendMessage(data, client.uniqueid)
		}
	}
	s.lker.Unlock()
}
