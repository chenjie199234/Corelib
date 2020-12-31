package web

import "fmt"

type WebConfig struct {
	SelfName          string
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

func checkconfig(c *WebConfig) {
	if c.ReadHeaderTimeout == 0 {
		fmt.Println("[Web.checkconfig]missing ReadHeaderTimeout,default will be:100ms")
		c.ReadHeaderTimeout = 100
	}
	if c.ReadTimeout == 0 {
		fmt.Println("[Web.checkconfig]missing ReadTimeout,default will be:200ms")
		c.ReadTimeout = 150
	}
	if c.WriteTimeout == 0 {
		fmt.Println("[Web.checkconfig]missing WriteTimeout,default will be:250ms")
		c.ReadTimeout = 250
	}
	if c.IdleTimeout == 0 {
		//if idletimeout is zero the readtimeout will be used
		fmt.Printf("[Web.checkconfig]missing idletimeout,ReadTimeout will be used to it:%d bytes\n", c.ReadTimeout)
	}
	if c.MaxHeaderBytes == 0 {
		fmt.Println("[Web.checkconfig]missing MaxHeaderBytes,default will be:512 bytes")
		c.MaxHeaderBytes = 512
	}
	if c.SocketReadBufferLen == 0 {
		fmt.Println("[Web.checkconfig]missing SocketReadBufferLen,default will be:1024 bytes")
		c.SocketReadBufferLen = 1024
	}
	if c.SocketWriteBufferLen == 0 {
		fmt.Println("[Web.checkconfig]missing SocketWriteBufferLen,default will be:1024 bytes")
		c.SocketWriteBufferLen = 1024
	}
}
