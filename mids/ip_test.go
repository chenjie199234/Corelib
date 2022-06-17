package mids

import (
	"testing"
)

func Test_Ip(t *testing.T) {
	UpdateIpConfig([]string{"192.168.0.0/24", "192.168.1.1"}, []string{"192.168.3.0/16", "192.167.0.1"})
	if !WhiteIP("192.168.1.1") {
		panic("should be white ip")
	}
	if !WhiteIP("192.168.0.230") {
		panic("should be white ip")
	}
	if WhiteIP("192.168.1.2") {
		panic("should not be white ip")
	}
	if !BlackIP("192.167.0.1") {
		panic("should be black ip")
	}
	if !BlackIP("192.168.4.10") {
		panic("should be black ip")
	}
	if BlackIP("192.167.1.1") {
		panic("should not be black ip")
	}
}
