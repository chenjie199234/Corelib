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
	reader     *list.BlockList[*Msg_Body]
	sender     func(context.Context, *Msg) error
	status     atomic.Int32 //use right 4 bit,the bit from left to right:peer_read_status,peer_send_status,self_read_status,self_send_status
	e          error
}

func newrw(callid uint64, path string, deadline int64, md, td map[string]string, sender func(context.Context, *Msg) error) *rw {
	tmp := &rw{
		callid:     callid,
		path:       path,
		metadata:   md,
		traceddata: td,
		deadline:   deadline,
		reader:     list.NewBlockList[*Msg_Body](),
		sender:     sender,
	}
	tmp.status.Store(0b1111)
	return tmp
}
func (this *rw) init(mb *Msg_Body) error {
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_INIT)
	mh.SetDeadline(this.deadline)
	mh.SetMetadata(this.metadata)
	mh.SetTracedata(this.traceddata)
	m.SetH(mh)
	m.SetB(mb)
	return this.sender(context.Background(), m)
}

// return io.EOF means peer stop recv
// return cerror.ErrCanceled means self stop send
// return cerror.DeadlineExceeded means timeout
// return cerror.ErrClosed means connection closed
// return cerror.ErrServerClosing
// return cerror.ErrRespmsgLen/cerror.ErrReqmsgLen
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
func (this *rw) send(ctx context.Context, mb *Msg_Body) error {
	if this.e != nil {
		return this.e
	}
	if this.status.Load()&0b0001 == 0 {
		return cerror.ErrCanceled
	}
	if this.status.Load()&0b1000 == 0 {
		return io.EOF
	}
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_SEND)
	m.SetH(mh)
	m.SetB(mb)
	if e := this.sender(ctx, m); e != nil {
		return e
	}
	if mb.GetError() != nil {
		//if we send error to peer,means we stop send
		this.status.And(0b1110)
	}
	return nil
}

func (this *rw) closesend() error {
	if old := this.status.And(0b1110); old&0b0001 == 0 {
		return nil
	}
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_CLOSE_SEND)
	m.SetH(mh)
	return this.sender(context.Background(), m)
}
func (this *rw) closerecv() error {
	if old := this.status.And(0b1101); old&0b0010 == 0 {
		return nil
	}
	this.reader.Close()
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_CLOSE_RECV)
	m.SetH(mh)
	return this.sender(context.Background(), m)
}
func (this *rw) closerecvsend() error {
	this.status.And(0b1100)
	this.reader.Close()
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_CLOSE_RECV_SEND)
	m.SetH(mh)
	return this.sender(context.Background(), m)
}

// return io.EOF means peer stop send
// return cerror.ErrCanceled means self stop recv
// return cerror.DeadlineExceeded means timeout
// return cerror.ErrClosed means connection closed
// return cerror.ErrServerClosing
func (this *rw) recv(ctx context.Context) ([]byte, Encoder, error) {
	m, e := this.reader.Pop(ctx)
	if e != nil {
		if e == list.ErrClosed {
			if this.e != nil {
				return nil, Encoder_UNKNOWN, this.e
			}
			if this.status.Load()&0b0100 == 0 {
				return nil, Encoder_UNKNOWN, io.EOF
			}
			if this.status.Load()&0b0010 == 0 {
				return nil, Encoder_UNKNOWN, cerror.ErrCanceled
			}
			//this is impossible
			return nil, Encoder_UNKNOWN, cerror.ErrClosed
		} else {
			//context.Canceled or context.DeadlineExceeded
			if this.e != nil {
				return nil, Encoder_UNKNOWN, this.e
			}
			return nil, Encoder_UNKNOWN, cerror.Convert(e)
		}
	}
	if m.GetError() == nil || m.GetError().GetCode() == 0 {
		return m.GetBody(), m.GetBodyEncoder(), nil
	}
	//if we read error from peer,means peer stop send
	this.status.And(0b1011)
	this.reader.Close()
	return nil, Encoder_UNKNOWN, m.GetError()
}
func (this *rw) cache(mb *Msg_Body) error {
	_, e := this.reader.Push(mb)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	return e
}
