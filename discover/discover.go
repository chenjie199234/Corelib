package discover

type PortType int

const (
	NotNeed PortType = iota
	Crpc
	Cgrpc
	Web
)

// version can only be int64 or string(should only be used with != or ==)
type Version any

type DI interface {
	// triger discover action once right now.
	// one triger must have one notice!
	Now()
	// notice will be trigered when discover action finished.
	// don't close the returned channel.
	// the channel will be closed when this discover stopped or the cancel function be called
	GetNotice() (notice <-chan *struct{}, cancel func())
	// lasterror will not be nil when the last discover action failed.
	// addrs and lasterror can both exist,when lasterror is not nil,the addrs is the old addrs.
	// version can only be int64 or string(should only be used with != or ==)
	GetAddrs(PortType) (addrs map[string]*RegisterData, version Version, lasterror error)
	Stop()
	CheckTarget(string) bool
}

type RegisterData struct {
	//app node register on which discovery server.
	//if this is empty means this app node is offline.
	DServers map[string]*struct{}
}

func SameVersion(aversion, bversion Version) bool {
	switch aversion.(type) {
	case string:
		switch bversion.(type) {
		case string:
			if aversion.(string) == bversion.(string) {
				return true
			}
		default:
		}
	case int64:
		switch bversion.(type) {
		case int64:
			if aversion.(int64) == bversion.(int64) {
				return true
			}
		default:
		}
	}
	return false
}
