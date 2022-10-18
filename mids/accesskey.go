package mids

type accesskey struct {
	seckeys map[string][]string //key path,value seckey
}

var accesskeyInstance *accesskey

func init() {
	accesskeyInstance = &accesskey{
		seckeys: make(map[string][]string),
	}
}

// seckeys's map key is path
// seckeys's map value is the seckey
func UpdateAccessKeyConfig(seckeys map[string][]string) {
	accesskeyInstance.seckeys = seckeys
}
func AccessKey(path string, key string) bool {
	seckey, ok := accesskeyInstance.seckeys[path]
	if !ok {
		seckey, ok = accesskeyInstance.seckeys["default"]
		if !ok {
			return false
		}
	}
	for _, v := range seckey {
		if v == key {
			return true
		}
	}
	return false
}
