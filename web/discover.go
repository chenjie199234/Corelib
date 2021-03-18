package web

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

func defaultDiscover(group, name string, client *WebClient) {
	notice := discovery.NoticeWebChanges(group + "." + name)

	currentinfos := []byte("{}")
	currentaddition := []byte(nil)
	for {
		<-notice
		infos, addition := discovery.GetWebInfos(group + "." + name)
		for _, v := range infos {
			sort.Strings(v)
		}
		tempinfos, _ := json.Marshal(infos)
		if !bytes.Equal(currentinfos, tempinfos) || !bytes.Equal(currentaddition, addition) {
			currentinfos = tempinfos
			currentaddition = addition
			log.Info("[web.client.defaultDiscover] update server:", group+"."+name, "addr:", common.Byte2str(currentinfos), "addition:", common.Byte2str(currentaddition))
			client.UpdateDiscovery(infos, addition)
		}
	}
}
