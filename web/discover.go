package web

import (
	"bytes"
	"encoding/json"
	"time"
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
	tker := time.NewTicker(client.c.DiscoverInterval)
	for {
		select {
		case <-tker.C:
		case <-client.manually:
		}
		all, e := client.c.DiscoverFunction(group, name)
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
		tker.Reset(client.c.DiscoverInterval)
		for len(tker.C) > 0 {
			<-tker.C
		}
	}
}
