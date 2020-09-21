package discovery

import (
	"bytes"
	"context"
	"encoding/json"
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
	allclients map[string]*clientnode //key peeruniquename
	verifydata []byte
	nodepool   *sync.Pool
	instance   *stream.Instance
}

type clientnode struct {
	peeruniquename string
	peer           *stream.Peer
	uniqueid       uint64
	regdata        []byte
	status         int //1 connected,2 preparing,3 registered
}

var serverinstance *server

func (s *server) getnode(peer *stream.Peer, peeruniquename string, uniqueid uint64) *clientnode {
	node := s.nodepool.Get().(*clientnode)
	node.peeruniquename = peeruniquename
	node.peer = peer
	node.uniqueid = uniqueid
	node.status = 1
	return node
}
func (s *server) putnode(n *clientnode) {
	n.peeruniquename = ""
	n.peer = nil
	n.uniqueid = 0
	n.regdata = nil
	n.status = 0
	s.nodepool.Put(n)
}

func StartDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	serverinstance = &server{
		lker:       &sync.RWMutex{},
		htree:      hashtree.New(10, 3),
		allclients: make(map[string]*clientnode),
		verifydata: vdata,
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &clientnode{}
			},
		},
	}
	//tcp instance
	c.Verifyfunc = serverinstance.verifyfunc
	c.Onlinefunc = serverinstance.onlinefunc
	c.Userdatafunc = serverinstance.userfunc
	c.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance = stream.NewInstance(c)
	go serverinstance.instance.StartTcpServer(cc, listenaddr)
}

func (s *server) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

func (s *server) onlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	s.lker.Lock()
	if _, ok := s.allclients[peeruniquename]; ok {
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.onlinefunc.impossible]duplicate connection from peer:%s\n", peeruniquename)
		return
	}
	s.allclients[peeruniquename] = s.getnode(p, peeruniquename, uniqueid)
	s.lker.Unlock()
}
func (s *server) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case mSGONLINE:
		_, regmsg, _, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s online message:%s broken\n", peeruniquename, data)
			p.Close(uniqueid)
			return
		}
		if e := json.Unmarshal(regmsg, &RegMsg{}); e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s online message:%s broken\n", peeruniquename, regmsg)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
		s.lker.Lock()
		node, ok := s.allclients[peeruniquename]
		if !ok {
			//this is impossible
			s.lker.Unlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", peeruniquename)
			p.Close(uniqueid)
			return
		}
		node.regdata = regmsg
		node.status = 3
		leaf, _ := s.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := []*clientnode{node}
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(peeruniquename[:strings.Index(peeruniquename, ":")] + byte2str(regmsg)),
				Value:   unsafe.Pointer(&leafdata),
			})
		} else {
			leafdata := *(*[]*clientnode)(leaf.Value)
			leafdata = append(leafdata, node)
			sort.Slice(leafdata, func(i, j int) bool {
				return leafdata[i].peeruniquename < leafdata[j].peeruniquename
			})
			all := make([]string, len(leafdata))
			for i, client := range leafdata {
				all[i] = client.peeruniquename[:strings.Index(client.peeruniquename, ":")] + byte2str(client.regdata)
			}
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(&leafdata),
			})
		}
		onlinemsg := makeOnlineMsg(peeruniquename, regmsg, s.htree.GetRootHash())
		//notice all other peers
		for _, client := range s.allclients {
			if client.status > 1 {
				client.peer.SendMessage(onlinemsg, client.uniqueid)
			}
		}
		s.lker.Unlock()
	case mSGPULL:
		s.lker.RLock()
		node, ok := s.allclients[peeruniquename]
		if !ok {
			//this is impossible
			s.lker.RUnlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", peeruniquename)
			p.Close(uniqueid)
			return
		}
		if node.status < 2 {
			node.status = 2
		}
		all := make(map[string][]byte, int(float64(len(s.allclients))*1.3))
		for peeruniquename, client := range s.allclients {
			if client.status == 3 {
				all[peeruniquename] = client.regdata
			}
		}
		p.SendMessage(makePushMsg(all), uniqueid)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc.impossible]unknown message type from peer:%s\n", peeruniquename)
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peeruniquename string) {
	leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	node, ok := s.allclients[peeruniquename]
	if !ok {
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.offlinefunc.impossible]peer:%s missing\n", peeruniquename)
		return
	}
	delete(s.allclients, peeruniquename)
	if node.status != 3 {
		s.putnode(node)
		return
	}
	leaf, _ := s.htree.GetLeaf(leafindex)
	if leaf == nil {
		//this is impossible
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.offlinefunc.impossible]peer:%s hashtree data missing\n", peeruniquename)
		s.putnode(node)
		return
	}
	leafdata := *(*[]*clientnode)(leaf.Value)
	for i, client := range leafdata {
		if client.peeruniquename == peeruniquename {
			leafdata = append(leafdata[:i], leafdata[i+1:]...)
			s.putnode(node)
			break
		}
	}
	if len(leafdata) == 0 {
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: nil,
			Value:   nil,
		})
	} else {
		all := make([]string, len(leafdata))
		for i, client := range leafdata {
			all[i] = client.peeruniquename[:strings.Index(client.peeruniquename, ":")] + byte2str(client.regdata)
		}
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: str2byte(strings.Join(all, "")),
			Value:   unsafe.Pointer(&leafdata),
		})
	}
	//notice all other peer
	offlinemsg := makeOfflineMsg(peeruniquename, s.htree.GetRootHash())
	for _, client := range s.allclients {
		if client.status > 1 {
			client.peer.SendMessage(offlinemsg, client.uniqueid)
		}
	}
	s.lker.Unlock()
}
