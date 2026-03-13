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
	//use right 5 bit,the bit from left to right
	//connection_status,peer_read_status,peer_send_status,self_read_status,self_send_status
	status    atomic.Int32
	peererror *cerror.Error
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
	tmp.status.Store(0b11111)
	return tmp
}
func (this *rw) init(ctx context.Context, mb *Msg_Body) error {
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
	return this.sender(ctx, m)
}

// return io.EOF means peer stop recv
// return cerror.ErrCanceled means self stop send
// return cerror.DeadlineExceeded means timeout
// return cerror.ErrClosed means connection closed
// return cerror.ErrRespmsgLen/cerror.ErrReqmsgLen
// Send will not wait peer to confirm accept the message,so there may be data lost if peer closed and self send at the same time
// if mb.GetError() != nil,it is same as closesend
func (this *rw) send(ctx context.Context, mb *Msg_Body) error {
	if this.status.Load()&0b10000 == 0 {
		return cerror.ErrClosed
	}
	if this.status.Load()&0b00001 == 0 {
		return cerror.ErrCanceled
	}
	if this.status.Load()&0b01000 == 0 {
		return io.EOF
	}
	m := &Msg{}
	mh := &Msg_Header{}
	mh.SetCallid(this.callid)
	mh.SetPath(this.path)
	mh.SetType(MsgType_SEND)
	m.SetH(mh)
	m.SetB(mb)
	return this.sender(ctx, m)
}

func (this *rw) closesend() error {
	if old := this.status.And(0b11110); old&0b00001 == 0 {
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
	if old := this.status.And(0b11101); old&0b00010 == 0 {
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
	if old := this.status.And(0b11100); old&0b00011 == 0 {
		return nil
	}
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
// other errors are user's errors
func (this *rw) recv(ctx context.Context) ([]byte, Encoder, error) {
	m, e := this.reader.Pop(ctx)
	if e != nil {
		if this.status.Load()&0b10000 == 0 {
			return nil, Encoder_UNKNOWN, cerror.ErrClosed
		}
		if e == list.ErrClosed {
			if this.status.Load()&0b00100 == 0 {
				return nil, Encoder_UNKNOWN, io.EOF
			}
			if this.status.Load()&0b00010 == 0 {
				return nil, Encoder_UNKNOWN, cerror.ErrCanceled
			}
			//this is impossible
			return nil, Encoder_UNKNOWN, cerror.ErrClosed
		} else {
			//context.Canceled or context.DeadlineExceeded
			return nil, Encoder_UNKNOWN, cerror.Convert(e)
		}
	}
	if m.GetError() == nil || m.GetError().GetCode() == 0 {
		return m.GetBody(), m.GetBodyEncoder(), nil
	}
	return nil, Encoder_UNKNOWN, m.GetError()
}
func (this *rw) cache(mb *Msg_Body) error {
	_, e := this.reader.Push(mb)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	//if we get an error from peer,means peer stop send
	if mb.GetError() != nil {
		this.status.And(0b11011)
		this.reader.Close()
		this.peererror = mb.GetError()
	}
	return e
}
