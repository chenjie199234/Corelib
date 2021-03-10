package web

import (
	"time"

	"github.com/chenjie199234/Corelib/discovery"
)

func defaultDiscover(group, name string, client *WebClient) {
	for {
		var notice chan struct{}
		var e error
		for {
			notice, e = discovery.NoticeWebChanges(group + "." + name)
			if e == nil {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		<-notice
		infos, addition := discovery.GetWebInfos(group + "." + name)
		client.UpdateDiscovery(infos, addition)
	}
}
