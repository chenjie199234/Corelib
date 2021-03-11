package discovery

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/log"
)

func defaultfinder(manually chan struct{}) {
	group := os.Getenv("DiscoveryServerGroup")
	if group == "" {
		panic("[Discovery.client.defaultfinder] missing system env DiscoveryServerGroup")
	}
	name := os.Getenv("DiscoveryServerName")
	if name == "" {
		panic("[Discovery.client.defaultfinder] missing system env DiscoveryServerName")
	}
	port := os.Getenv("DiscoveryServerPort")
	n, e := strconv.Atoi(port)
	if e != nil {
		panic("[Discovery.client.defaultfinder] missing system env DiscoveryServerPort or not number")
	}
	if n <= 0 || n > 65535 {
		panic("[Discovery.client.defaultfinder] system env DiscoveryServerPort out of range,(0-65535)")
	}
	host := name + "-service." + group
	servername := group + "." + name
	finder := func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		addrs, e := net.DefaultResolver.LookupHost(ctx, host)
		if e != nil {
			log.Error("[Discovery.client.defaultfinder] dns resolve host:", host, "error:", e)
			return
		}
		for i, addr := range addrs {
			addrs[i] = servername + ":" + addr + ":" + port
		}
		log.Info("[Discovery.client.defaultfinder] dns resolve host:", host, "success:", addrs)
		UpdateDiscoveryServers(addrs)
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
