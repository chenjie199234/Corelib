package mids

import (
	"encoding/hex"
	"hash"

	"github.com/chenjie199234/Corelib/util/common"
)

type access struct {
	seckeys map[string]map[string]string //first key path,second key accessid,value accesskey
}

var accessInstance *access

func init() {
	accessInstance = &access{
		seckeys: make(map[string]map[string]string),
	}
}

// first key path,second key secid,value seckey
func UpdateAccessKeyConfig(seckeys map[string]map[string]string) {
	accessInstance.seckeys = seckeys
}

func AccessKeyCheck(path string, accesskey string) bool {
	sec, ok := accessInstance.seckeys[path]
	if !ok {
		sec, ok = accessInstance.seckeys["default"]
		if !ok {
			return false
		}
	}
	for _, key := range sec {
		if key == accesskey {
			return true
		}
	}
	return false
}
func AccessSignCheck(path, accessid, data, sign string, hs []hash.Hash) bool {
	sec, ok := accessInstance.seckeys[path]
	if !ok {
		sec, ok = accessInstance.seckeys["default"]
		if !ok {
			return false
		}
	}
	accesskey, ok := sec[accessid]
	if !ok {
		return false
	}
	origin := path + data + accesskey
	for _, h := range hs {
		origin = hex.EncodeToString(h.Sum(common.Str2byte(origin)))
	}
	return origin == sign
}
func AccessSignMake(path, accesskey, data string, hs []hash.Hash) string {
	origin := path + data + accesskey
	for _, h := range hs {
		origin = hex.EncodeToString(h.Sum(common.Str2byte(origin)))
	}
	return origin
}
