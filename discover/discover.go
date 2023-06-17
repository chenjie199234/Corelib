package discover

type PortType int

const (
	NotNeed PortType = iota
	Crpc
	Cgrpc
	Web
)

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
	GetAddrs(PortType) (addrs map[string]*RegisterData, lasterror error)
	Stop()
	CheckApp(string) bool
}

type RegisterData struct {
	//app node register on which discovery server.
	//if this is empty means this app node is offline.
	DServers map[string]*struct{}
	Addition []byte
}
