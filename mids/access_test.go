package mids

import "testing"
import "crypto/md5"
import "crypto/sha256"
import "hash"

func Test_Accesskey(t *testing.T) {
	UpdateAccessConfig(map[string]map[string]string{"/abc": {"1": "1", "2": "2"}})
	if AccessKeyCheck("/a", "abc") {
		panic("should not pass the access key check")
	}
	if AccessKeyCheck("/abc", "123") {
		panic("should not pass the access key check")
	}
	if !AccessKeyCheck("/abc", "1") {
		panic("should pass the access key check")
	}
	if !AccessKeyCheck("/abc", "2") {
		panic("should pass the access key check")
	}
}
func Test_Accesssign(t *testing.T) {
	UpdateAccessConfig(map[string]map[string]string{"/abc": {"1": "1", "2": "2"}})
	s := AccessSignMake("/abc", "1", "test", []hash.Hash{md5.New(), sha256.New()})
	if !AccessSignCheck("/abc", "1", "test", s, []hash.Hash{md5.New(), sha256.New()}) {
		panic("should pass the access sign check")
	}
	if AccessSignCheck("/abc", "1", "test", "asdlasdj", []hash.Hash{md5.New(), sha256.New()}) {
		panic("should not pass the access sign check")
	}
	if AccessSignCheck("/abc", "1", "asldjlasd", s, []hash.Hash{md5.New(), sha256.New()}) {
		panic("should not pass the access sign check")
	}
}
