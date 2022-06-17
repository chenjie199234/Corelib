package mids

import "testing"

func Test_Accesskey(t *testing.T) {
	UpdateAccessKeyConfig(map[string][]string{"/abc": {"111", "222"}})
	if AccessKey("/a", "abc") {
		panic("should not pass the access key check")
	}
	if AccessKey("/abc", "abc") {
		panic("should not pass the access key check")
	}
	if !AccessKey("/abc", "111") {
		panic("should pass the access key check")
	}
	if !AccessKey("/abc", "222") {
		panic("should pass the access key check")
	}
}
