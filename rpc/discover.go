package rpc

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"github.com/chenjie199234/Corelib/discovery"
	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

func defaultDiscover(group, name string, client *RpcClient) {
	var notice chan struct{}
	var e error
	for {
		notice, e = discovery.NoticeRpcChanges(group + "." + name)
		if e == nil {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	currentinfos := []byte("{}")
	currentaddition := []byte(nil)
	for {
		<-notice
		infos, addition := discovery.GetRpcInfos(group + "." + name)
		for _, v := range infos {
			sort.Strings(v)
		}
		tempinfos, _ := json.Marshal(infos)
		if !bytes.Equal(currentinfos, tempinfos) || !bytes.Equal(currentaddition, addition) {
			currentinfos = tempinfos
			currentaddition = addition
			log.Info("[rpc.client.defaultDiscover] update server:", group+"."+name, "addr:", common.Byte2str(currentinfos), "addition:", common.Byte2str(currentaddition))
			client.UpdateDiscovery(infos, addition)
		}
	}
}
