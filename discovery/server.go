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
	count      uint64
}

type serverhashtreeleafdata struct {
	clientsindex []string
	clients      map[string]*clientnode
}

type clientnode struct {
	peer     *stream.Peer
	uniqueid uint64
	regdata  []byte
}

var serverinstance *server

func (s *server) getnode(peer *stream.Peer, regdata []byte, uniqueid uint64) *clientnode {
	node := s.nodepool.Get().(*clientnode)
	node.peer = peer
	node.uniqueid = uniqueid
	node.regdata = regdata
	return node
}
func (s *server) putnode(n *clientnode) {
	n.peer = nil
	n.uniqueid = 0
	n.regdata = nil
	s.nodepool.Put(n)
}

func StartDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	if serverinstance != nil {
		return
	}
	serverinstance = &server{
		lker:  &sync.RWMutex{},
		htree: hashtree.New(10, 3),
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &clientnode{}
			},
		},
		verifydata: vdata,
	}
	//instance
	c.Verifyfunc = serverinstance.verifyfunc
	//c.Onlinefunc = instance.onlinefunc
	c.Userdatafunc = serverinstance.userfunc
	c.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance = stream.NewInstance(c)
	serverinstance.instance.StartTcpServer(cc, listenaddr)
}

func (s *server) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

//func (s *server) onlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
//}
func (s *server) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case mSGONLINE:
		_, regmsg, _, e := getOnlineMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.server.userfunc]online message:%s broken\n", data)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
		s.lker.Lock()
		leaf, _ := s.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := &serverhashtreeleafdata{
				clientsindex: []string{peeruniquename},
				clients:      map[string]*clientnode{peeruniquename: s.getnode(p, regmsg, uniqueid)},
			}
			name := peeruniquename[:strings.Index(peeruniquename, ":")]
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(name + byte2str(regmsg)),
				Value:   unsafe.Pointer(leafdata),
			})
		} else {
			leafdata := (*serverhashtreeleafdata)(leaf.Value)
			if _, ok := leafdata.clients[peeruniquename]; ok {
				s.lker.Unlock()
				//this is impossible
				fmt.Printf("[Discovery.server.userfunc]duplicate connection,peeruniquename:%s\n", peeruniquename)
				p.Close(uniqueid)
				return
			}
			leafdata.clients[peeruniquename] = s.getnode(p, regmsg, uniqueid)
			leafdata.clientsindex = append(leafdata.clientsindex, peeruniquename)
			sort.Strings(leafdata.clientsindex)
			all := make([]string, len(leafdata.clientsindex))
			for i, indexname := range leafdata.clientsindex {
				client := leafdata.clients[indexname]
				all[i] = indexname[:strings.Index(indexname, ":")] + byte2str(client.regdata)
			}
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		onlinemsg := makeOnlineMsg(peeruniquename, regmsg, s.htree.GetRootHash())
		leaves := s.htree.GetAllLeaf()
		//notice all other peers
		for _, leaf := range leaves {
			leafdata := (*serverhashtreeleafdata)(leaf.Value)
			for _, client := range leafdata.clients {
				client.peer.SendMessage(onlinemsg, client.uniqueid)
			}
		}
		s.count++
		s.lker.Unlock()
	case mSGPULL:
		s.lker.RLock()
		leaves := s.htree.GetAllLeaf()
		all := make(map[string][]byte, int(float64(s.count)*1.3))
		for _, leaf := range leaves {
			leafdata := (*serverhashtreeleafdata)(leaf.Value)
			for peeruniquename, client := range leafdata.clients {
				all[peeruniquename] = client.regdata
			}
		}
		p.SendMessage(makePushMsg(all), uniqueid)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type\n")
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peeruniquename string) {
	leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	leaf, _ := s.htree.GetLeaf(leafindex)
	if leaf == nil {
		s.lker.Unlock()
		return
	}
	leafdata := (*serverhashtreeleafdata)(leaf.Value)
	if client, ok := leafdata.clients[peeruniquename]; !ok {
		s.lker.Unlock()
		//this is impossible
		fmt.Printf("[Discovery.server.offlinefunc]duplicate connection,peeruniquename:%s\n", peeruniquename)
		return
	} else {
		delete(leafdata.clients, peeruniquename)
		s.putnode(client)
		for i, clientname := range leafdata.clientsindex {
			if clientname == peeruniquename {
				leafdata.clientsindex = append(leafdata.clientsindex[:i], leafdata.clientsindex[i+1:]...)
				break
			}
		}
	}
	all := make([]string, len(leafdata.clientsindex))
	for i, indexname := range leafdata.clientsindex {
		client, _ := leafdata.clients[indexname]
		all[i] = indexname[:strings.Index(indexname, ":")] + byte2str(client.regdata)
	}
	if len(all) == 0 {
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: hashtree.Emptyhash[:],
			Value:   nil,
		})
	} else {
		s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
			Hashstr: str2byte(strings.Join(all, "")),
			Value:   unsafe.Pointer(leafdata),
		})
	}
	s.count--
	//notice all other peer
	offlinemsg := makeOfflineMsg(peeruniquename, s.htree.GetRootHash())
	leaves := s.htree.GetAllLeaf()
	for _, leaf := range leaves {
		leafdata := (*serverhashtreeleafdata)(leaf.Value)
		for _, client := range leafdata.clients {
			client.peer.SendMessage(offlinemsg, client.uniqueid)
		}
	}
	s.lker.Unlock()
}
