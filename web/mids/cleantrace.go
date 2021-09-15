package mids

import (
	"github.com/chenjie199234/Corelib/web"
)

func CleanTrace(ctx *web.Context) {
	headers := ctx.GetHeaders()
	delete(headers, "Traceid")
	delete(headers, "Tracedata")
	delete(headers, "Metadata")
	delete(headers, "SourceServer")
	delete(headers, "Deadline")
}
