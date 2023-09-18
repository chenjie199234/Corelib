package crpc

import (
	"context"
	"github.com/chenjie199234/Corelib/cerror"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/internal/picker"
	"github.com/chenjie199234/Corelib/stream"

	"google.golang.org/protobuf/proto"
)

type ServerForPick struct {
	callid   uint64 //start from 100
	addr     string
	dservers map[string]*struct{} //this app registered on which discovery server
	peer     *stream.Peer
	closing  bool

	//active calls
	lker *sync.Mutex
	reqs map[uint64]*req //all reqs to this server

	Pickinfo *picker.ServerPickInfo
}

func (s *ServerForPick) GetServerPickInfo() *picker.ServerPickInfo {
	return s.Pickinfo
}

func (s *ServerForPick) getpeer() *stream.Peer {
	return (*stream.Peer)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer))))
}
func (s *ServerForPick) setpeer(p *stream.Peer) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(p))
}
func (s *ServerForPick) caspeer(oldpeer, newpeer *stream.Peer) bool {
	return atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.peer)), unsafe.Pointer(&oldpeer), unsafe.Pointer(newpeer))
}
func (s *ServerForPick) Pickable() bool {
	return s.getpeer() != nil && !s.closing
}

func (s *ServerForPick) sendmessage(ctx context.Context, r *req) (e error) {
	p := s.getpeer()
	if p == nil {
		return cerror.ErrClosed
	}
	beforeSend := func(_ *stream.Peer) {
		s.lker.Lock()
	}
	afterSend := func(_ *stream.Peer, e error) {
		if e != nil {
			s.lker.Unlock()
			return
		}
		s.reqs[r.reqdata.Callid] = r
		s.lker.Unlock()
	}
	d, _ := proto.Marshal(r.reqdata)
	if e = p.SendMessage(ctx, d, beforeSend, afterSend); e != nil {
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
			e = cerror.ConvertStdError(e)
		}
		return
	}
	return
}
func (s *ServerForPick) sendcancel(ctx context.Context, canceldata []byte) {
	if p := s.getpeer(); p != nil {
		p.SendMessage(ctx, canceldata, nil, nil)
	}
}
