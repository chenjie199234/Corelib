package crpc

import (
	"context"
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
	readstatus int32
	sendstatus int32
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
		readstatus: 1, //this will be 0 when recv the CloseSend msg from peer or call the closeread function by self
		sendstatus: 1, //this will be 0 when recv the CloseRead msg from peer or call the closesend function by self
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
func (this *rw) write(ctx context.Context, body *MsgBody) error {
	if this.sendstatus == 0 {
		return cerror.ErrStreamSendClosed
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
func (this *rw) closesend(ctx context.Context) error {
	if atomic.SwapInt32(&this.sendstatus, 0) != 1 {
		return nil
	}
	return this.sender(ctx, &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseSend,
		},
	})
}
func (this *rw) closeread(ctx context.Context) error {
	if atomic.SwapInt32(&this.readstatus, 0) != 1 {
		return nil
	}
	this.reader.Close()
	return this.sender(ctx, &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseRead,
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
	m, e := this.reader.Pop(ctx)
	if e != nil {
		if e == list.ErrClosed {
			if this.readstatus == 0 {
				return nil, nil, cerror.ErrStreamReadClosed
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
	return m.Body, m.Traildata, m.Error
}
func (this *rw) cache(m *MsgBody) error {
	_, e := this.reader.Push(m)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	return e
}
