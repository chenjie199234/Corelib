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

type clientnode struct {
	peer     *stream.Peer
	name     string
	grpcaddr string
	httpaddr string
	tcpaddr  string
	uniqueid uint64
}

var serverinstance *server

func (s *server) getnode(peer *stream.Peer, name, grpcaddr, httpaddr, tcpaddr string, uniqueid uint64) *clientnode {
	node := s.nodepool.Get().(*clientnode)
	node.peer = peer
	node.name = name
	node.grpcaddr = grpcaddr
	node.httpaddr = httpaddr
	node.tcpaddr = tcpaddr
	node.uniqueid = uniqueid
	return node
}
func (s *server) putnode(n *clientnode) {
	n.peer = nil
	n.name = ""
	n.grpcaddr = ""
	n.httpaddr = ""
	n.tcpaddr = ""
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

func (s *server) verifyfunc(ctx context.Context, peeruniquename string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}
func (s *server) onlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
}
func (s *server) userfunc(p *stream.Peer, peeruniquename string, uniqueid uint64, data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case MSGREG:
		regmsg, e := getRegMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.server.userfunc]get regmsg error:%s\n", e)
			p.Close(uniqueid)
			return
		}
		leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
		s.lker.Lock()
		leaf, _ := s.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := &hashtreeleafdata{
				clientsindex: []string{peeruniquename},
				clients:      map[string]*clientnode{},
			}
			name := peeruniquename[:strings.Index(peeruniquename, ":")]
			leafdata.clients[peeruniquename] = s.getnode(p, name, regmsg.GrpcAddr, regmsg.HttpAddr, regmsg.TcpAddr, uniqueid)
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(name + regmsg.GrpcAddr + regmsg.HttpAddr + regmsg.TcpAddr),
				Value:   unsafe.Pointer(leafdata),
			})
		} else {
			leafdata := (*hashtreeleafdata)(leaf.Value)
			if _, ok := leafdata.clients[peeruniquename]; ok {
				s.lker.Unlock()
				//this is impossible
				fmt.Printf("[Discovery.server.userfunc]duplicate connection,peeruniquename:%s\n", peeruniquename)
				p.Close(uniqueid)
				return
			}
			name := peeruniquename[:strings.Index(peeruniquename, ":")]
			leafdata.clients[peeruniquename] = s.getnode(p, name, regmsg.GrpcAddr, regmsg.HttpAddr, regmsg.TcpAddr, uniqueid)
			leafdata.clientsindex = append(leafdata.clientsindex, peeruniquename)
			sort.Strings(leafdata.clientsindex)
			all := make([]string, len(leafdata.clientsindex))
			for i, key := range leafdata.clientsindex {
				client, _ := leafdata.clients[key]
				all[i] = client.name + client.grpcaddr + client.httpaddr + client.tcpaddr
			}
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		onlinemsg := makeOnlineMsg(peeruniquename, data[1:], s.htree.GetRootHash())
		leaves := s.htree.GetAllLeaf()
		//notice all other peers
		for _, leaf := range leaves {
			leafdata := (*hashtreeleafdata)(leaf.Value)
			for _, client := range leafdata.clients {
				client.peer.SendMessage(onlinemsg, client.uniqueid)
			}
		}
		s.count++
		s.lker.Unlock()
	case MSGPULL:
		s.lker.RLock()
		leaves := s.htree.GetAllLeaf()
		all := make(map[string]*RegMsg, int(float64(s.count)*1.3))
		for _, leaf := range leaves {
			leafdata := (*hashtreeleafdata)(leaf.Value)
			for peeruniquename, client := range leafdata.clients {
				all[peeruniquename] = &RegMsg{
					GrpcAddr: client.grpcaddr,
					HttpAddr: client.httpaddr,
					TcpAddr:  client.tcpaddr,
				}
			}
		}
		p.SendMessage(makePushMsg(all), uniqueid)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type\n")
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	leafindex := int(bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	leaf, _ := s.htree.GetLeaf(leafindex)
	leafdata := (*hashtreeleafdata)(leaf.Value)
	if client, ok := leafdata.clients[peeruniquename]; !ok || client.uniqueid != uniqueid {
		s.lker.Unlock()
		//this is impossible
		fmt.Printf("[Discovery.server.offlinefunc]duplicate connection,peeruniquename:%s\n", peeruniquename)
		return
	} else {
		delete(leafdata.clients, peeruniquename)
		s.putnode(client)
	}
	for i, clientname := range leafdata.clientsindex {
		if clientname == peeruniquename {
			leafdata.clientsindex = append(leafdata.clientsindex[:i], leafdata.clientsindex[i+1:]...)
			break
		}
	}
	all := make([]string, len(leafdata.clientsindex))
	for i, key := range leafdata.clientsindex {
		client, _ := leafdata.clients[key]
		all[i] = client.name + client.grpcaddr + client.httpaddr + client.tcpaddr
	}
	s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
		Hashstr: str2byte(strings.Join(all, "")),
		Value:   unsafe.Pointer(leafdata),
	})
	//notice all other peer
	offlinemsg := makeOfflineMsg(peeruniquename, s.htree.GetRootHash())
	leaves := s.htree.GetAllLeaf()
	for _, leaf := range leaves {
		leafdata := (*hashtreeleafdata)(leaf.Value)
		for _, client := range leafdata.clients {
			client.peer.SendMessage(offlinemsg, client.uniqueid)
		}
	}
	s.lker.Unlock()
}
