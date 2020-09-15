package server

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/discovery/msg"
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

type hashtreeleafdata struct {
	clientsindex []string
	clients      map[string]*clientnode
}

type clientnode struct {
	peer     *stream.Peer
	uniqueid uint64
	msg.RegMsg
}

var instance *server

func (s *server) getnode(peer *stream.Peer, grpcaddr, httpaddr, tcpaddr string, uniqueid uint64) *clientnode {
	node := s.nodepool.Get().(*clientnode)
	node.peer = peer
	node.uniqueid = uniqueid
	node.GrpcAddr = grpcaddr
	node.HttpAddr = httpaddr
	node.TcpAddr = tcpaddr
	return node
}
func (s *server) putnode(n *clientnode) {
	n.peer = nil
	n.uniqueid = 0
	n.GrpcAddr = ""
	n.HttpAddr = ""
	n.TcpAddr = ""
	s.nodepool.Put(n)
}

func StartDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	if instance != nil {
		return
	}
	instance = &server{
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
	c.Verifyfunc = instance.verifyfunc
	//c.Onlinefunc = instance.onlinefunc
	c.Userdatafunc = instance.userfunc
	c.Offlinefunc = instance.offlinefunc
	instance.instance = stream.NewInstance(c)
	instance.instance.StartTcpServer(cc, listenaddr)
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
	case msg.MSGREG:
		regmsg, e := msg.GetRegMsg(data)
		if e != nil {
			fmt.Printf("[Discovery.server.userfunc]reg message broken,error:%s\n", e)
			p.Close(uniqueid)
			return
		}
		leafindex := int(msg.Bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
		s.lker.Lock()
		leaf, _ := s.htree.GetLeaf(leafindex)
		if leaf == nil {
			leafdata := &hashtreeleafdata{
				clientsindex: []string{peeruniquename},
				clients:      map[string]*clientnode{},
			}
			name := peeruniquename[:strings.Index(peeruniquename, ":")]
			leafdata.clients[peeruniquename] = s.getnode(p, regmsg.GrpcAddr, regmsg.HttpAddr, regmsg.TcpAddr, uniqueid)
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: msg.Str2byte(name + regmsg.GrpcAddr + regmsg.HttpAddr + regmsg.TcpAddr),
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
			leafdata.clients[peeruniquename] = s.getnode(p, regmsg.GrpcAddr, regmsg.HttpAddr, regmsg.TcpAddr, uniqueid)
			leafdata.clientsindex = append(leafdata.clientsindex, peeruniquename)
			sort.Strings(leafdata.clientsindex)
			all := make([]string, len(leafdata.clientsindex))
			for i, indexname := range leafdata.clientsindex {
				client := leafdata.clients[indexname]
				all[i] = indexname[:strings.Index(indexname, ":")] + client.GrpcAddr + client.HttpAddr + client.TcpAddr
			}
			s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
				Hashstr: msg.Str2byte(strings.Join(all, "")),
				Value:   unsafe.Pointer(leafdata),
			})
		}
		onlinemsg := msg.MakeOnlineMsg(peeruniquename, data[1:], s.htree.GetRootHash())
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
	case msg.MSGPULL:
		s.lker.RLock()
		leaves := s.htree.GetAllLeaf()
		all := make(map[string]*msg.RegMsg, int(float64(s.count)*1.3))
		for _, leaf := range leaves {
			leafdata := (*hashtreeleafdata)(leaf.Value)
			for peeruniquename, client := range leafdata.clients {
				all[peeruniquename] = &msg.RegMsg{
					GrpcAddr: client.GrpcAddr,
					HttpAddr: client.HttpAddr,
					TcpAddr:  client.TcpAddr,
				}
			}
		}
		p.SendMessage(msg.MakePushMsg(all), uniqueid)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type\n")
		p.Close(uniqueid)
	}
}
func (s *server) offlinefunc(p *stream.Peer, peeruniquename string, uniqueid uint64) {
	leafindex := int(msg.Bkdrhash(peeruniquename, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	leaf, _ := s.htree.GetLeaf(leafindex)
	if leaf == nil {
		s.lker.Unlock()
		return
	}
	leafdata := (*hashtreeleafdata)(leaf.Value)
	if client, ok := leafdata.clients[peeruniquename]; !ok || client.uniqueid != uniqueid {
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
		all[i] = indexname[:strings.Index(indexname, ":")] + client.GrpcAddr + client.HttpAddr + client.TcpAddr
	}
	s.htree.SetSingleLeaf(leafindex, &hashtree.LeafData{
		Hashstr: msg.Str2byte(strings.Join(all, "")),
		Value:   unsafe.Pointer(leafdata),
	})
	//notice all other peer
	offlinemsg := msg.MakeOfflineMsg(peeruniquename, s.htree.GetRootHash())
	leaves := s.htree.GetAllLeaf()
	for _, leaf := range leaves {
		leafdata := (*hashtreeleafdata)(leaf.Value)
		for _, client := range leafdata.clients {
			client.peer.SendMessage(offlinemsg, client.uniqueid)
		}
	}
	s.lker.Unlock()
}
