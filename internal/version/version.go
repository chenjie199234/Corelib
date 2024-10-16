package version

import "fmt"

var major = 0
var minor = 0
var patch = 130
var status = ""

func String() string {
	if status != "" {
		return fmt.Sprintf("v%d.%d.%d-%s", major, minor, patch, status)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch)
}
