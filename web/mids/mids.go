package mids

import (
	"github.com/chenjie199234/Corelib/web"
)

//dosn't include global mids in here
var all map[string]web.OutsideHandler

func init() {
	all = make(map[string]web.OutsideHandler)
	//register here
	all["clean"] = Clean
}

func AllMids() map[string]web.OutsideHandler {
	return all
}
