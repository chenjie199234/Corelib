package stream

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"unsafe"

	"github.com/chenjie199234/Corelib/util/common"
)

type Instance struct {
	selfname string
	c        *InstanceConfig

	tcplistener *net.TCPListener
	mng         *connmng
}

//be careful about the callback func race
func NewInstance(c *InstanceConfig, group, name string) (*Instance, error) {
	if e := common.NameCheck(name, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group, false, true, false, true); e != nil {
		return nil, e
	}
	if e := common.NameCheck(group+"."+name, true, true, false, true); e != nil {
		return nil, e
	}
	if c == nil {
		return nil, errors.New("[Stream.NewInstance] config is nil")
	}
	//verify func can't be nill
	//user data deal func can't be nill
	//online and offline func can be nill
	if c.Verifyfunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing verify function")
	}
	if c.Userdatafunc == nil {
		return nil, errors.New("[Stream.NewInstance] missing userdata function")
	}
	c.validate()
	stream := &Instance{
		selfname: group + "." + name,
		c:        c,
		mng:      newconnmng(int(c.GroupNum), c.HeartprobeInterval, c.SendIdleTimeout, c.RecvIdleTimeout),
	}
	return stream, nil
}
func (this *Instance) Stop() {
	this.mng.Stop()
	tmplistener := (*net.TCPListener)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&this.tcplistener))))
	if tmplistener != nil {
		this.tcplistener.Close()
	}
}
func (this *Instance) GetSelfName() string {
	return this.selfname
}
func (this *Instance) GetPeerNum() int32 {
	return this.mng.GetPeerNum()
}
func (this *Instance) SendMessageAll(ctx context.Context, data []byte, beforeSend func(*Peer), afterSend func(*Peer, error)) {
	this.mng.SendMessage(ctx, data, beforeSend, afterSend)
}
