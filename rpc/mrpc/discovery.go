package mrpc

import (
	"fmt"
	"time"

	"github.com/chenjie199234/Corelib/discovery"
)

func defaultdiscovery(appname string, client *MrpcClient) {
	for {
		var notice chan struct{}
		var e error
		for {
			notice, e = discovery.NoticeTcpChanges(appname)
			if e == nil {
				break
			}
			fmt.Printf("[Mrpc.client.defaultdiscovery]Notice discovery changes error:%s\n", e)
			time.Sleep(time.Millisecond * 100)
		}
		<-notice
		infos := discovery.GetTcpInfos(appname)
		client.UpdateDiscovery(infos)
	}
}
