package crpc

import (
	"context"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/stream"

	"google.golang.org/protobuf/proto"
)

type ServerForPick struct {
	callid   uint64 //start from 100
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server
	peer     *stream.Peer
	closing  int32

	//active calls
	lker *sync.RWMutex
	rws  map[uint64]*rw //all calls to this server

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}
func (s *ServerForPick) GetServerAddr() string {
	return s.addr
}
func (s *ServerForPick) Pickable() bool {
	return s.peer != nil && s.closing == 0
}

func (s *ServerForPick) getpeer() *stream.Peer {
	return (*stream.Peer)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer))))
}
func (s *ServerForPick) setpeer(p *stream.Peer) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(p))
}
func (s *ServerForPick) caspeer(oldpeer, newpeer *stream.Peer) bool {
	return atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(oldpeer), unsafe.Pointer(newpeer))
}

func (s *ServerForPick) updateRegister(dservers map[string]*struct{}) {
	//unregister on which discovery server
	dserveroffline := false
	for dserver := range s.dservers {
		if _, ok := dservers[dserver]; !ok {
			dserveroffline = true
			break
		}
	}
	//register on which new discovery server
	for dserver := range dservers {
		if _, ok := s.dservers[dserver]; !ok {
			dserveroffline = false
			break
		}
	}
	s.dservers = dservers
	if dserveroffline {
		s.Pickinfo.SetDiscoverServerOffline(uint32(len(dservers)))
	} else {
		s.Pickinfo.SetDiscoverServerOnline(uint32(len(dservers)))
	}
}
func (s *ServerForPick) unRegister() {
	s.dservers = nil
	s.Pickinfo.SetDiscoverServerOffline(0)
}
func (s *ServerForPick) createrw(path string, deadline int64, md, td map[string]string) *rw {
	callid := atomic.AddUint64(&s.callid, 1)
	rw := newrw(callid, path, deadline, md, td, s.sendmessage)
	s.lker.Lock()
	defer s.lker.Unlock()
	s.rws[callid] = rw
	return rw
}
func (s *ServerForPick) getrw(callid uint64) *rw {
	s.lker.RLock()
	defer s.lker.RUnlock()
	return s.rws[callid]
}
func (s *ServerForPick) closerw(callid uint64) {
	s.lker.Lock()
	defer s.lker.Unlock()
	rw, ok := s.rws[callid]
	if ok {
		rw.reader.Close()
	}
	delete(s.rws, callid)
}
func (s *ServerForPick) cleanrw() {
	s.lker.Lock()
	for callid, rw := range s.rws {
		rw.reader.Close()
		delete(s.rws, callid)
	}
	s.lker.Unlock()
}
func (s *ServerForPick) sendmessage(ctx context.Context, m *Msg) (e error) {
	p := s.getpeer()
	if p == nil {
		return cerror.ErrClosed
	}
	d, _ := proto.Marshal(m)
	if e = p.SendMessage(ctx, d, nil, nil); e != nil {
		if e == stream.ErrMsgLarge {
			e = cerror.ErrReqmsgLen
		} else if e == stream.ErrConnClosed {
			e = cerror.ErrClosed
			s.caspeer(p, nil)
		} else if e == context.DeadlineExceeded {
			e = cerror.ErrDeadlineExceeded
		} else if e == context.Canceled {
			e = cerror.ErrCanceled
		} else {
			//this is impossible
			e = cerror.Convert(e)
		}
	}
	return
}
