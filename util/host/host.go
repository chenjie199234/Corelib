package host

import (
	"os"
)

var Hostip string
var Hostname string

func init() {
	Hostname = os.Getenv("HOSTNAME")
	if Hostname == "" {
		Hostname = "unknown"
	}
	if Hostip = os.Getenv("HOSTIP"); Hostip == "" {
		if Hostip = os.Getenv("HOST_IP"); Hostip == "" {
			if Hostip = os.Getenv("PODIP"); Hostip == "" {
				if Hostip = os.Getenv("POD_IP"); Hostip == "" {
					if Hostip = os.Getenv("LOCALIP"); Hostip == "" {
						Hostip = os.Getenv("LOCAL_IP")
					}
				}
			}
		}
	}
	if Hostip == "" {
		Hostip = "unknown"
	}
}
