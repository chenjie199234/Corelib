package crpc

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/container/list"
)

type rw struct {
	callid     uint64
	path       string
	metadata   map[string]string
	traceddata map[string]string
	deadline   int64
	reader     *list.BlockList[*MsgBody]
	sender     func(context.Context, *Msg) error
	status     int32 //use right 4 bit,the bit from left to right:peer_read_status,peer_send_status,self_read_status,self_send_status
}

func newrw(callid uint64, path string, deadline int64, md, td map[string]string, sender func(context.Context, *Msg) error) *rw {
	return &rw{
		callid:     callid,
		path:       path,
		metadata:   md,
		traceddata: td,
		deadline:   deadline,
		reader:     list.NewBlockList[*MsgBody](),
		sender:     sender,
		status:     0b1111,
	}
}
func (this *rw) init(ctx context.Context, body *MsgBody) error {
	return this.sender(ctx, &Msg{
		H: &MsgHeader{
			Callid:    this.callid,
			Path:      this.path,
			Type:      MsgType_Init,
			Deadline:  this.deadline,
			Tracedata: this.traceddata,
		},
		B:     body,
		WithB: body != nil,
	})
}
func (this *rw) send(ctx context.Context, body *MsgBody) error {
	if this.status&0b0001 == 0 {
		return cerror.ErrCanceled
	}
	if this.status&0b1000 == 0 {
		return io.EOF
	}
	return this.sender(ctx, &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_Send,
		},
		B:     body,
		WithB: body != nil,
	})
}
func (this *rw) closesend() error {
	if old := atomic.AndInt32(&this.status, 0b1110); old&0b0001 == 0 {
		return nil
	}
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseSend,
		},
	})
}
func (this *rw) closeread() error {
	if old := atomic.AndInt32(&this.status, 0b1101); old&0b0010 == 0 {
		return nil
	}
	this.reader.Close()
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseRead,
		},
	})
}
func (this *rw) closereadwrite() error {
	atomic.AndInt32(&this.status, 0b1100)
	this.reader.Close()
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseReadSend,
		},
	})
}
func (this *rw) cancel() error {
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_Cancel,
		},
	})
}
func (this *rw) read(ctx context.Context) ([]byte, map[string]string, error) {
	if this.status&0b0010 == 0 {
		return nil, nil, cerror.ErrCanceled
	}
	m, e := this.reader.Pop(ctx)
	if e != nil {
		if e == list.ErrClosed {
			if this.status&0b0100 == 0 {
				return nil, nil, io.EOF
			}
			if this.status&0b0010 == 0 {
				return nil, nil, cerror.ErrCanceled
			}
			return nil, nil, cerror.ErrClosed
		} else if e == context.DeadlineExceeded {
			return nil, nil, cerror.ErrDeadlineExceeded
		} else if e == context.Canceled {
			return nil, nil, cerror.ErrCanceled
		} else {
			//this is impossible
			return nil, nil, cerror.Convert(e)
		}
	}
	//fix interface nil problem
	if m.Error == nil {
		return m.Body, m.Traildata, nil
	}
	//if we read error from peer,means peer stop send
	atomic.AndInt32(&this.status, 0b1011)
	return m.Body, m.Traildata, m.Error
}
func (this *rw) cache(m *MsgBody) error {
	_, e := this.reader.Push(m)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	return e
}
