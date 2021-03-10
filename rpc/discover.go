package rpc

import (
	"time"

	"github.com/chenjie199234/Corelib/discovery"
)

func defaultdiscovery(group, name string, client *RpcClient) {
	for {
		var notice chan struct{}
		var e error
		for {
			notice, e = discovery.NoticeRpcChanges(group + "." + name)
			if e == nil {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		<-notice
		infos, addition := discovery.GetRpcInfos(group + "." + name)
		client.UpdateDiscovery(infos, addition)
	}
}
