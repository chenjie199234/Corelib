package mids

import (
	"github.com/chenjie199234/Corelib/crpc"
)

//dosn't include global mids in here
var all map[string]crpc.OutsideHandler

func init() {
	all = make(map[string]crpc.OutsideHandler)
	//register here
	//e.g.
	//all["auth"] = auth.MidwareHandler
	//all["limit"] = limit.MidwareHandler
}

func AllMids() map[string]crpc.OutsideHandler {
	return all
}
