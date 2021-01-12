package configcenter

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/chenjie199234/Corelib/stream"
)

var (
	ERRSINIT    = fmt.Errorf("[Configcenter.server]not init,call NewConfigServer first")
	ERRSSTARTED = fmt.Errorf("[Configcenter.server]already started")
)

type configserver struct {
	c          *stream.InstanceConfig
	verifydata []byte
	instance   *stream.Instance
	status     int32
}

var serverinstance *configserver

func NewConfigServer(c *stream.InstanceConfig, vdata []byte, url string) {
	if serverinstance != nil {
		return
	}
	serverinstance = &configserver{
		verifydata: vdata,
	}
	//websocket instance
	dupc := *c //duplicate to remote the callback func race
	dupc.Verifyfunc = serverinstance.verifyfunc
	dupc.Onlinefunc = serverinstance.onlinefunc
	dupc.Userdatafunc = serverinstance.userfunc
	dupc.Offlinefunc = serverinstance.offlinefunc
	serverinstance.c = &dupc
	serverinstance.instance = stream.NewInstance(&dupc)
}
func StartConfigServer(cc *stream.WebConfig, path, listenaddr string) error {
	if serverinstance == nil {
		return ERRSINIT
	}
	if old := atomic.SwapInt32(&serverinstance.status, 1); old == 1 {
		return ERRSSTARTED
	}
	go serverinstance.instance.StartWebsocketServer(cc, []string{path}, listenaddr, func(*http.Request) bool { return true })
	return nil
}
func (s *configserver) verifyfunc(ctx context.Context, clientuniquename string, peerVerifyData []byte) ([]byte, bool) {
	datas := strings.Split(byte2str(peerVerifyData), "|")
	if len(datas) != 2 {
		return nil, false
	}
	if datas[1] != s.c.SelfName || hex.EncodeToString(s.verifydata) != datas[0] {
		return nil, false
	}
	return s.verifydata, true
}
func (s *configserver) onlinefunc(p *stream.Peer, clientuniquename string, starttime uint64) {

}
func (s *configserver) userfunc(p *stream.Peer, clientuniquename string, data []byte, starttime uint64) {

}
func (s *configserver) offlinefunc(p *stream.Peer, clientuniquename string) {

}
