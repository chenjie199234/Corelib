package web

type WebConfig struct {
	Addr              string
	ReadHeaderTimeout int //millisecond
	ReadTimeout       int //millisecond
	WriteTimeout      int //millisecond
	IdleTimeout       int //millisecond
	MaxHeaderBytes    int
	//socket
	SocketReadBufferLen  int
	SocketWriteBufferLen int
	//https
	TlsCertFile string
	TlsKeyFile  string
}
