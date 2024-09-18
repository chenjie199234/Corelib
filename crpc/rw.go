package crpc

import (
	"context"
	"io"
	"strconv"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/cerror"
	"github.com/chenjie199234/Corelib/container/list"
	"github.com/chenjie199234/Corelib/monitor"
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
	e          error
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
	if this.status&0b0001 == 0 {
		if this.e != nil {
			return this.e
		}
		return cerror.ErrCanceled
	}
	if this.status&0b1000 == 0 {
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
		atomic.AndInt32(&this.status, 0b1110)
	}
	return nil
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
func (this *rw) closerecv() error {
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
func (this *rw) closerecvsend(trail bool, e error) error {
	if old := atomic.AndInt32(&this.status, 0b1100); old&0b0011 == 0 {
		return nil
	}
	this.e = e
	this.reader.Close()
	m := &Msg{
		H: &MsgHeader{
			Callid: this.callid,
			Path:   this.path,
			Type:   MsgType_CloseReadSend,
		},
	}
	if trail {
		m.H.Traildata = map[string]string{"Cpu-Usage": strconv.FormatFloat(monitor.LastUsageCPU, 'g', 10, 64)}
	}
	return this.sender(context.Background(), m)
}
func (this *rw) recv() ([]byte, error) {
	if this.status&0b0010 == 0 {
		if this.e != nil {
			return nil, this.e
		}
		return nil, cerror.ErrCanceled
	}
	m, e := this.reader.Pop(context.Background())
	if e != nil {
		if e == list.ErrClosed {
			if this.e != nil {
				return nil, this.e
			}
			if this.status&0b0100 == 0 {
				return nil, io.EOF
			}
			if this.status&0b0010 == 0 {
				return nil, cerror.ErrCanceled
			}
			return nil, cerror.ErrClosed
		} else {
			//this is impossible
			return nil, cerror.Convert(e)
		}
	}
	//fix interface nil problem
	if m.Error == nil || m.Error.Code == 0 {
		return m.Body, nil
	}
	//if we read error from peer,means peer stop send
	atomic.AndInt32(&this.status, 0b1011)
	this.reader.Close()
	return m.Body, m.Error
}
func (this *rw) cache(m *MsgBody) error {
	_, e := this.reader.Push(m)
	if e == list.ErrClosed {
		e = cerror.ErrClosed
	}
	return e
}
