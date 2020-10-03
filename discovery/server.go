package discovery

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenjie199234/Corelib/hashtree"
	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRSINIT    = fmt.Errorf("[Discovery.server]not init,call NewDiscoveryServer first")
	ERRSSTARTED = fmt.Errorf("[Discovery.server]already started")
)

//clientuniquename = appname:addr
type discoveryserver struct {
	c          *stream.InstanceConfig
	lker       *sync.RWMutex
	htree      *hashtree.Hashtree
	allclients map[string]*discoveryclientnode //key clientuniquename
	verifydata []byte
	nodepool   *sync.Pool
	instance   *stream.Instance
	status     int32
}

//clientuniquename = appname:addr
type discoveryclientnode struct {
	clientuniquename string
	peer             *stream.Peer
	starttime        uint64
	regdata          []byte
	status           int //1 connected,2 preparing,3 registered
}

func (s *discoveryserver) getnode(peer *stream.Peer, clientuniquename string, starttime uint64) *discoveryclientnode {
	node := s.nodepool.Get().(*discoveryclientnode)
	node.clientuniquename = clientuniquename
	node.peer = peer
	node.starttime = starttime
	node.status = 1
	return node
}

func (s *discoveryserver) putnode(n *discoveryclientnode) {
	n.clientuniquename = ""
	n.peer = nil
	n.starttime = 0
	n.regdata = nil
	n.status = 0
	s.nodepool.Put(n)
}

var serverinstance *discoveryserver

func NewDiscoveryServer(c *stream.InstanceConfig, vdata []byte) {
	if serverinstance != nil {
		return
	}
	serverinstance = &discoveryserver{
		c:          c,
		lker:       &sync.RWMutex{},
		htree:      hashtree.New(10, 3),
		allclients: make(map[string]*discoveryclientnode),
		verifydata: vdata,
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &discoveryclientnode{}
			},
		},
	}
	//tcp instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = serverinstance.onlinefunc
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = serverinstance.offlinefunc
	serverinstance.instance = stream.NewInstance(&dupc)
}
func StartDiscoveryServer(cc *stream.TcpConfig, listenaddr string) error {
	if serverinstance == nil {
		return ERRSINIT
	}
	serverinstance.lker.Lock()
	if serverinstance.status >= 1 {
		serverinstance.lker.Unlock()
		return ERRSSTARTED
	}
	serverinstance.status = 1
	serverinstance.lker.Unlock()
	serverinstance.instance.StartTcpServer(cc, listenaddr)
	return nil
}

func (s *discoveryserver) verifyfunc(ctx context.Context, clientuniquename string, peerVerifyData []byte) ([]byte, bool) {
	//datas := bytes.Split(peerVerifyData, []byte{'|'})
	datas := strings.Split(byte2str(peerVerifyData), "|")
	if len(datas) != 2 {
		return nil, false
	}
	if datas[1] != s.c.SelfName || datas[0] != hex.EncodeToString(s.verifydata) {
		return nil, false
	}
	return s.verifydata, true
}

func (s *discoveryserver) onlinefunc(p *stream.Peer, clientuniquename string, starttime uint64) {
	s.lker.Lock()
	if _, ok := s.allclients[clientuniquename]; ok {
		s.lker.Unlock()
		//this is impossible
		fmt.Printf("[Discovery.server.onlinefunc.impossible]duplicate connection from peer:%s\n", clientuniquename)
		return
	}
	s.allclients[clientuniquename] = s.getnode(p, clientuniquename, starttime)
	s.lker.Unlock()
}
func (s *discoveryserver) userfunc(p *stream.Peer, clientuniquename string, data []byte, starttime uint64) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case msgonline:
		_, regmsg, _, e := getOnlineMsg(data)
		if e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s online message:%s broken\n", clientuniquename, data)
			p.Close()
			return
		}
		reg := &RegMsg{}
		if e := json.Unmarshal(regmsg, reg); e != nil {
			//this is impossible
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s online message:%s broken\n", clientuniquename, regmsg)
			p.Close()
			return
		}
		ip := clientuniquename[strings.Index(clientuniquename, ":")+1 : strings.LastIndex(clientuniquename, ":")]
		if reg.GrpcPort != 0 && reg.GrpcIp == "" {
			reg.GrpcIp = ip
		}
		if reg.HttpPort != 0 && reg.HttpIp == "" {
			reg.HttpIp = ip
		}
		if reg.TcpPort != 0 && reg.TcpIp == "" {
			reg.TcpIp = ip
		}
		if reg.WebSockPort != 0 && reg.WebSockIp == "" {
			reg.WebSockIp = ip
		}
		regmsg, _ = json.Marshal(reg)
		leafindex := int(bkdrhash(clientuniquename, uint64(s.htree.GetLeavesNum())))
		s.lker.Lock()
		node, ok := s.allclients[clientuniquename]
		if !ok {
			//this is impossible
			s.lker.Unlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", clientuniquename)
			p.Close()
			return
		}
		node.regdata = regmsg
		node.status = 3
		templeafdata, _ := s.htree.GetLeafValue(leafindex)
		if templeafdata == nil {
			leafdata := []*discoveryclientnode{node}
			s.htree.SetSingleLeafHash(leafindex, str2byte(clientuniquename[:strings.Index(clientuniquename, ":")]+byte2str(regmsg)))
			s.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		} else {
			leafdata := *(*[]*discoveryclientnode)(templeafdata)
			leafdata = append(leafdata, node)
			sort.Slice(leafdata, func(i, j int) bool {
				return leafdata[i].clientuniquename < leafdata[j].clientuniquename
			})
			all := make([]string, len(leafdata))
			for i, client := range leafdata {
				all[i] = client.clientuniquename[:strings.Index(client.clientuniquename, ":")] + byte2str(client.regdata)
			}
			s.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
			s.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
		}
		onlinemsg := makeOnlineMsg(clientuniquename, regmsg, s.htree.GetRootHash())
		//notice all other peers
		for _, client := range s.allclients {
			if client.status > 1 {
				client.peer.SendMessage(onlinemsg, client.starttime)
			}
		}
		s.lker.Unlock()
	case msgpull:
		s.lker.RLock()
		node, ok := s.allclients[clientuniquename]
		if !ok {
			//this is impossible
			s.lker.RUnlock()
			fmt.Printf("[Discovery.server.userfunc.impossible]peer:%s missing\n", clientuniquename)
			p.Close()
			return
		}
		if node.status < 2 {
			node.status = 2
		}
		all := make(map[string][]byte, int(float64(len(s.allclients))*1.3))
		for clientuniquename, client := range s.allclients {
			if client.status == 3 {
				all[clientuniquename] = client.regdata
			}
		}
		p.SendMessage(makePushMsg(all), starttime)
		s.lker.RUnlock()
	default:
		fmt.Printf("[Discovery.server.userfunc.impossible]unknown message type from peer:%s\n", clientuniquename)
		p.Close()
	}
}
func (s *discoveryserver) offlinefunc(p *stream.Peer, clientuniquename string) {
	leafindex := int(bkdrhash(clientuniquename, uint64(s.htree.GetLeavesNum())))
	s.lker.Lock()
	node, ok := s.allclients[clientuniquename]
	if !ok {
		s.lker.Unlock()
		fmt.Printf("[Discovery.server.offlinefunc.impossible]peer:%s missing\n", clientuniquename)
		return
	}
	delete(s.allclients, clientuniquename)
	if node.status != 3 {
		s.putnode(node)
		s.lker.Unlock()
		return
	}
	templeafdata, _ := s.htree.GetLeafValue(leafindex)
	leafdata := *(*[]*discoveryclientnode)(templeafdata)
	for i, client := range leafdata {
		if client.clientuniquename == clientuniquename {
			leafdata = append(leafdata[:i], leafdata[i+1:]...)
			break
		}
	}
	s.putnode(node)
	if len(leafdata) == 0 {
		s.htree.SetSingleLeafHash(leafindex, nil)
		s.htree.SetSingleLeafValue(leafindex, nil)
	} else {
		all := make([]string, len(leafdata))
		for i, client := range leafdata {
			all[i] = client.clientuniquename[:strings.Index(client.clientuniquename, ":")] + byte2str(client.regdata)
		}
		s.htree.SetSingleLeafHash(leafindex, str2byte(strings.Join(all, "")))
		s.htree.SetSingleLeafValue(leafindex, unsafe.Pointer(&leafdata))
	}
	//notice all other peer
	offlinemsg := makeOfflineMsg(clientuniquename, s.htree.GetRootHash())
	for _, client := range s.allclients {
		if client.status > 1 {
			client.peer.SendMessage(offlinemsg, client.starttime)
		}
	}
	s.lker.Unlock()
}
