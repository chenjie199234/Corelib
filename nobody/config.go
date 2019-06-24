package nobody

type Config struct {
	//common
	Addr              string
	ReadHeaderTimeout int //millisecond
	ReadTimeout       int //millisecond
	WriteTimeout      int //millisecond
	IdleTimeout       int //millisecond
	MaxHeaderBytes    int
	KeepAlive         bool
	//https
	CertFilePath string
	KeyFilePath  string
}
