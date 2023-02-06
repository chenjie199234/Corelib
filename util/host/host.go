package host

import (
	"os"
)

var Hostip string
var Hostname string

func init() {
	if Hostname = os.Getenv("HOSTNAME"); Hostname == "" {
		if Hostname = os.Getenv("HOST_NAME"); Hostname == "" {
			if Hostname = os.Getenv("PODNAME"); Hostname == "" {
				if Hostname = os.Getenv("POD_NAME"); Hostname == "" {
					if Hostname = os.Getenv("LOCALNAME"); Hostname == "" {
						Hostname = os.Getenv("LOCAL_NAME")
					}
				}
			}
		}
	}
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
