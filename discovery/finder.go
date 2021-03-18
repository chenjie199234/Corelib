package discovery

import (
	"context"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/util/common"
)

func MakeDefaultFinder(servergroup, servername string, serverport int) func(manually chan struct{}) {
	if e := common.NameCheck(servergroup, false, true, false, true); e != nil {
		log.Error("[Discovery.client.MakeDefaultFinder] error:", e)
		return nil
	}
	if e := common.NameCheck(servername, false, true, false, true); e != nil {
		log.Error("[Discovery.client.MakeDefaultFinder] error:", e)
		return nil
	}
	if e := common.NameCheck(servergroup+"."+servername, true, true, false, true); e != nil {
		log.Error("[Discovery.client.MakeDefaultFinder] error:", e)
		return nil
	}
	if serverport <= 0 || serverport > 65535 {
		log.Error("[Discovery.client.MakeDefaultFinder] discovery server port out of range")
		return nil
	}
	return func(manually chan struct{}) {
		host := servername + "-service." + servergroup
		appname := servergroup + "." + servername

		current := make([]string, 0)

		finder := func() {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
			defer cancel()
			addrs, e := net.DefaultResolver.LookupHost(ctx, host)
			if e != nil {
				log.Error("[Discovery.client.DefaultFinder] dns resolve host:", host, "error:", e)
				return
			}
			if len(addrs) != 0 {
				sort.Strings(addrs)
				for i, addr := range addrs {
					addrs[i] = appname + ":" + addr + ":" + strconv.Itoa(serverport)
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
				log.Info("[Discovery.client.DefaultFinder] dns resolve host:", host, "result:", current)
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
}
