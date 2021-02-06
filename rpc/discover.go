package rpc

import (
	"time"

	"github.com/chenjie199234/Corelib/discovery"
)

func defaultdiscovery(appname string, client *MrpcClient) {
	for {
		var notice chan struct{}
		var e error
		for {
			notice, e = discovery.NoticeRpcChanges(appname)
			if e == nil {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		<-notice
		infos, addition := discovery.GetRpcInfos(appname)
		client.UpdateDiscovery(infos, addition)
	}
}
