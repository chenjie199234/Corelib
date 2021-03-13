package discovery

import (
	"context"
	"net"
	"os"
	"sort"
	"time"

	"github.com/chenjie199234/Corelib/log"
)

func defaultfinder(manually chan struct{}) {
	group := os.Getenv("DISCOVERY_SERVER_GROUP")
	name := os.Getenv("DISCOVERY_SERVER_NAME")
	port := os.Getenv("DISCOVERY_SERVER_PORT")
	host := name + "-service." + group
	servername := group + "." + name

	current := make([]string, 0)

	finder := func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		addrs, e := net.DefaultResolver.LookupHost(ctx, host)
		if e != nil {
			log.Error("[Discovery.client.defaultfinder] dns resolve host:", host, "error:", e)
			return
		}
		if len(addrs) == 0 {
			log.Error("[Discovery.client.defaultfinder] dns resolve host:", host, "empty result")
		} else {
			sort.Strings(addrs)
			for i, addr := range addrs {
				addrs[i] = servername + ":" + addr + ":" + port
			}
		}
		different := false
		if len(current) != len(addrs) {
			different = true
		} else {
			for i, addr := range addrs {
				if addr != current[i] {
					different = true
					break
				}
			}
		}
		if different {
			current = addrs
			log.Info("[Discovery.client.defaultfinder] dns resolve host:", host, "result:", current)
			UpdateDiscoveryServers(addrs)
		}
	}
	finder()
	tker := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-tker.C:
			finder()
		case <-manually:
			finder()
		}
	}
}
