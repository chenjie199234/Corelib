package web

import (
	"time"

	"github.com/chenjie199234/Corelib/discovery"
)

func defaultDiscover(appname string, client *WebClient) {
	for {
		var notice chan struct{}
		var e error
		for {
			notice, e = discovery.NoticeWebChanges(appname)
			if e == nil {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		<-notice
		infos, addition := discovery.GetWebInfos(appname)
		client.UpdateDiscovery(infos, addition)
	}
}
