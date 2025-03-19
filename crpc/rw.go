package crpc

import (
	"context"
	"io"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/container/list"
	"github.com/chenjie199234/Corelib/cotel"
)

type rw struct {
	callid     uint64
	path       string
	metadata   map[string]string
	traceddata map[string]string
	deadline   int64
	reader     *list.BlockList[*MsgBody]
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
		reader:     list.NewBlockList[*MsgBody](),
		sender:     sender,
	}
	tmp.status.Store(0b1111)
	return tmp
}
func (this *rw) init(body *MsgBody) error {
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid:    this.callid,
			Path:      this.path,
			Type:      MsgType_Init,
			Deadline:  this.deadline,
			Metadata:  this.metadata,
			Tracedata: this.traceddata,
		},
		B:     body,
		WithB: body != nil,
	})
}
func (this *rw) send(body *MsgBody) error {
	if this.status.Load()&0b0001 == 0 {
		if this.e != nil {
			return this.e
		}
		return cerror.ErrCanceled
	}
	if this.status.Load()&0b1000 == 0 {
		return io.EOF
	}
	if e := this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_Send,
		},
		B:     body,
		WithB: body != nil,
	}); e != nil {
		return e
	}
	if body.Error != nil {
		//if we send error to peer,means we stop send
		this.status.And(0b1110)
	}
	return nil
}
func (this *rw) closesend() error {
	if old := this.status.And(0b1110); old&0b0001 == 0 {
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
func (this *rw) closerecv() error {
	if old := this.status.And(0b1101); old&0b0010 == 0 {
		return nil
	}
	this.reader.Close()
	return this.sender(context.Background(), &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseRecv,
		},
	})
}
func (this *rw) closerecvsend(trail bool, err error) error {
	if old := this.status.And(0b1100); old&0b0011 == 0 {
		return nil
	}
	this.e = err
	this.reader.Close()
	m := &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseRecvSend,
		},
	}
	if trail {
		lastcpu, _, _ := cotel.GetCPU()
		m.H.Traildata = map[string]string{"Cpu-Usage": strconv.FormatFloat(lastcpu, 'g', 10, 64)}
	}
	return this.sender(context.Background(), m)
}
func (this *rw) recv() ([]byte, Encoder, error) {
	if this.status.Load()&0b0010 == 0 {
		if this.e != nil {
			return nil, Encoder_Unknown, this.e
		}
		return nil, Encoder_Unknown, cerror.ErrCanceled
	}
	m, e := this.reader.Pop(context.Background())
	if e != nil {
		if e == list.ErrClosed {
			if this.e != nil {
				return nil, Encoder_Unknown, this.e
			}
			if this.status.Load()&0b0100 == 0 {
				return nil, Encoder_Unknown, io.EOF
			}
			if this.status.Load()&0b0010 == 0 {
				return nil, Encoder_Unknown, cerror.ErrCanceled
			}
			return nil, Encoder_Unknown, cerror.ErrClosed
		} else {
			//this is impossible
			return nil, Encoder_Unknown, cerror.Convert(e)
		}
	}
	if m.Error == nil || m.Error.Code == 0 {
		return m.Body, m.BodyEncoder, nil
	}
	//if we read error from peer,means peer stop send
	this.status.And(0b1011)
	this.reader.Close()
	return nil, Encoder_Unknown, m.Error
}
func (this *rw) cache(m *MsgBody) error {
	_, e := this.reader.Push(m)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	return e
}
