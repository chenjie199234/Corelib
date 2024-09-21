package cgrpc

import (
	"io"
	"net/http"
	"strings"

	"github.com/chenjie199234/Corelib/cerror"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Warning!this function is only used for generated code,don't use it in any other place
// clientorserver:
// -- client true
// -- server false
func transGrpcError(e error, clientorserver bool) error {
	if e == io.EOF {
		return e
	}
	if ee, ok := e.(*cerror.Error); ok {
		return ee
	}
	s := status.Convert(e)
	if s == nil {
		return nil
	}
	switch s.Code() {
	case codes.OK:
		return nil
	case codes.Canceled:
		if strings.Contains(s.Message(), "is closing") {
			return cerror.ErrClosed
		}
		return cerror.ErrCanceled
	case codes.DeadlineExceeded:
		return cerror.ErrDeadlineExceeded
	case codes.Unknown:
		return cerror.Decode(s.Message())
	case codes.InvalidArgument:
		return cerror.ErrReq
	case codes.NotFound:
		return cerror.ErrNotExist
	case codes.AlreadyExists:
		return cerror.ErrAlreadyExist
	case codes.PermissionDenied:
		return cerror.ErrPermission
	case codes.ResourceExhausted:
		if strings.Contains(s.Message(), "received message") && strings.Contains(s.Message(), "larger") {
			if clientorserver {
				return cerror.ErrRespmsgLen
			}
			return cerror.ErrReqmsgLen
		} else if (strings.Contains(s.Message(), "send message") && strings.Contains(s.Message(), "larger")) || strings.Contains(s.Message(), "message too large") {
			if clientorserver {
				return cerror.ErrReqmsgLen
			}
			return cerror.ErrRespmsgLen
		}
		return cerror.MakeCError(-1, http.StatusInternalServerError, s.Message())
	case codes.FailedPrecondition:
		return cerror.MakeCError(-1, http.StatusInternalServerError, s.Message())
	case codes.Aborted:
		return cerror.MakeCError(-1, http.StatusInternalServerError, s.Message())
	case codes.OutOfRange:
		return cerror.MakeCError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unimplemented:
		return cerror.ErrNoapi
	case codes.Internal:
		return cerror.MakeCError(-1, http.StatusInternalServerError, s.Message())
	case codes.Unavailable:
		if strings.Contains(s.Message(), "EOF") {
			return io.EOF
		} else if strings.Contains(s.Message(), "is closing") ||
			(strings.Contains(s.Message(), "connection") && strings.Contains(s.Message(), "closed")) {
			return cerror.ErrClosed
		}
		return cerror.MakeCError(-1, http.StatusServiceUnavailable, s.Message())
	case codes.DataLoss:
		return cerror.MakeCError(-1, http.StatusNotFound, s.Message())
	case codes.Unauthenticated:
		return cerror.ErrAuth
	default:
		ee := cerror.Decode(s.Message())
		ee.SetHttpcode(int32(s.Code()))
		return ee
	}
}
