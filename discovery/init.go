package discovery

import (
	"net/http"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/hashtree"
)

func init() {
	clientinstance = &client{
		slker:   &sync.RWMutex{},
		servers: make(map[string]*servernode, 5),
		httpclient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
		nlker:       &sync.Mutex{},
		grpcnotices: make(map[string]chan *NoticeMsg, 2),
		httpnotices: make(map[string]chan *NoticeMsg, 2),
		tcpnotices:  make(map[string]chan *NoticeMsg, 2),
		webnotices:  make(map[string]chan *NoticeMsg, 2),
		status:      false,
	}
	serverinstance = &server{
		lker:  &sync.RWMutex{},
		htree: hashtree.New(10, 3),
		nodepool: &sync.Pool{
			New: func() interface{} {
				return &clientnode{}
			},
		},
		count:  0,
		status: false,
	}
}
