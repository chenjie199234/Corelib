package host

import (
	"os"
)

var Hostip string
var Hostname string
var Container bool

func init() {
	Hostname = os.Getenv("HOSTNAME")
	if Hostname == "" {
		Hostname = "unknown"
	}
	Hostip = os.Getenv("HOSTIP")
	if Hostip == "" {
		Hostip = "unknown"
	}
	_, Container = os.LookupEnv("KUBERNETES_SERVICE_HOST")
}
