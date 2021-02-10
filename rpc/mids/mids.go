package mids

import (
	"github.com/chenjie199234/Corelib/rpc"
)

//dosn't include global mids in here
var all map[string]rpc.OutsideHandler

func init() {
	all = make(map[string]rpc.OutsideHandler)
	//register here
	//e.g.
	//all["auth"] = auth.MidwareHandler
	//all["limit"] = limit.MidwareHandler
}

func AllMids() map[string]rpc.OutsideHandler {
	return all
}
