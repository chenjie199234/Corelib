package web

import (
	"bytes"
	"encoding/json"
)

func defaultDiscover(group, name string, client *WebClient) {
	notice := func() {
		client.mlker.Lock()
		for notice := range client.manualNotice {
			notice <- struct{}{}
			delete(client.manualNotice, notice)
		}
		client.mlker.Unlock()
	}
	var check []byte
	for {
		all, e := client.c.DiscoverFunction(group, name, client.manually)
		if e != nil {
			continue
		}
		d, _ := json.Marshal(all)
		if bytes.Equal(check, d) {
			notice()
			continue
		}
		check = d
		client.updateDiscovery(all)
		notice()
	}
}
