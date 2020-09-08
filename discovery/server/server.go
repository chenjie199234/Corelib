package server

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"google.golang.org/protobuf/proto"

	"github.com/chenjie199234/Corelib/buckettree"
	"github.com/chenjie199234/Corelib/discovery/msg"
	"github.com/chenjie199234/Corelib/stream"
)

var verifydata []byte

var lker *sync.RWMutex
var hashtree *buckettree.BucketTree
var nodes [][]*node
var nodepool *sync.Pool

type node struct {
	p *stream.Peer
	n string
	u uint64
}

func getnode(peer *stream.Peer, name string, uniqueid uint64) *node {
	result := nodepool.Get().(*node)
	result.n = name
	result.p = peer
	result.u = uniqueid
	return result
}
func putnode(n *node) {
	n.p = nil
	n.n = ""
	n.u = 0
	nodepool.Put(n)
}

//tcp instance
var instance *stream.Instance

func NewDiscoveryServer(c *stream.InstanceConfig, cc *stream.TcpConfig, listenaddr string, vdata []byte) {
	//verify
	verifydata = vdata
	//hash
	lker = &sync.RWMutex{}
	hashtree = buckettree.New(10, 2)
	nodes = make([][]*node, hashtree.GetBucketNum())
	for i := range nodes {
		nodes[i] = make([]*node, 0)
	}
	nodepool = &sync.Pool{
		New: func() interface{} {
			return &node{}
		},
	}
	//instance
	c.Verifyfunc = verifyfunc
	c.Onlinefunc = onlinefunc
	c.Userdatafunc = userfunc
	c.Offlinefunc = offlinefunc
	instance = stream.NewInstance(c)
	instance.StartTcpServer(cc, listenaddr)
}
func verifyfunc(ctx context.Context, peername string, uniqueid uint64, peerVerifyData []byte) ([]byte, bool) {
	if !bytes.Equal(peerVerifyData, verifydata) {
		return nil, false
	}
	return verifydata, true
}
func onlinefunc(p *stream.Peer, peername string, uniqueid uint64) {
	where := bkdrhash(peername)
	lker.Lock()
	nodes[where] = append(nodes[where], getnode(p, peername, uniqueid))
	sort.Slice(nodes[where], func(i, j int) bool {
		return nodes[where][i].n < nodes[where][j].n
	})
	all := make([]string, len(nodes[where]))
	for i, v := range nodes[where] {
		all[i] = v.n
	}
	hashtree.UpdateSingle(where, Str2byte(strings.Join(all, "")))
	data := msg.MakeOnlineMsg(peername, hashtree.GetRootHash())
	for _, v := range nodes {
		for _, vv := range v {
			vv.p.SendMessage(data, vv.u)
		}
	}
	lker.Unlock()
}
func userfunc(ctx context.Context, p *stream.Peer, peername string, uniqueid uint64, data []byte) {
	m := &msg.Totalmsg{}
	if e := proto.Unmarshal(data, m); e != nil {
		fmt.Printf("[Discovery.server.userfunc]unmarshal message error:%s\n", e)
		return
	}
	switch m.Mtype {
	case msg.MSGTYPE_PULL:
		lker.RLock()
		count := 0
		for _, v := range nodes {
			count += len(v)
		}
		result := make([]string, count)
		count = 0
		for _, v := range nodes {
			for _, vv := range v {
				result[count] = vv.n
				count++
			}
		}
		lker.RUnlock()
		data := msg.MakePushMsg(result)
		p.SendMessage(data, uniqueid)
	default:
		fmt.Printf("[Discovery.server.userfunc]unknown message type")
		p.Close(uniqueid)
	}
}
func offlinefunc(p *stream.Peer, peername string, uniqueid uint64) {
	where := bkdrhash(peername)
	lker.Lock()
	for i, v := range nodes[where] {
		if v.n == peername {
			nodes[where] = append(nodes[where][:i], nodes[where][i+1:]...)
			putnode(v)
			break
		}
	}
	all := make([]string, len(nodes[where]))
	for i, v := range nodes[where] {
		all[i] = v.n
	}
	hashtree.UpdateSingle(where, Str2byte(strings.Join(all, "")))
	data := msg.MakeOfflineMsg(peername, hashtree.GetRootHash())
	for _, v := range nodes {
		for _, vv := range v {
			vv.p.SendMessage(data, vv.u)
		}
	}
	lker.Unlock()
}
func bkdrhash(nameip string) uint64 {
	seed := uint64(131313)
	hash := uint64(0)
	for _, v := range nameip {
		hash = hash*seed + uint64(v)
	}
	return hash % hashtree.GetBucketNum()
}
func Str2byte(data string) []byte {
	temp := (*[2]uintptr)(unsafe.Pointer(&data))
	result := [3]uintptr{temp[0], temp[1], temp[1]}
	return *(*[]byte)(unsafe.Pointer(&result))
}
func Byte2str(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
