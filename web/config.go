package web

import (
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Timeout            time.Duration
	StaticFileRootPath string
	MaxHeader          int
	ReadBuffer         int //socket buffer
	WriteBuffer        int //socker buffer
	Cors               *CorsConfig
}

type CorsConfig struct {
	AllowedOrigin    []string
	AllowedHeader    []string
	ExposeHeader     []string
	AllowCredentials bool
	MaxAge           time.Duration
	allorigin        bool
	allheader        bool
	headerstr        string
	exposestr        string
}

func (c *Config) validate() {
	if c.Cors == nil {
		c.Cors = &CorsConfig{
			AllowedOrigin:    []string{"*"},
			AllowedHeader:    []string{"*"},
			ExposeHeader:     nil,
			AllowCredentials: false,
			MaxAge:           time.Hour * 24,
		}
	}
	if c.MaxHeader == 0 {
		c.MaxHeader = 1024
	}
	if c.ReadBuffer == 0 {
		c.ReadBuffer = 1024
	}
	if c.WriteBuffer == 0 {
		c.WriteBuffer = 1024
	}
	for _, v := range c.Cors.AllowedOrigin {
		if v == "*" {
			c.Cors.allorigin = true
			break
		}
	}
	hasorigin := false
	for _, v := range c.Cors.AllowedHeader {
		if v == "*" {
			c.Cors.allheader = true
			break
		} else if v == "Origin" {
			hasorigin = true
		}
	}
	if !c.Cors.allheader && !hasorigin {
		c.Cors.AllowedHeader = append(c.Cors.AllowedHeader, "Origin")
	}
	c.Cors.headerstr = c.getHeaders()
	c.Cors.exposestr = c.getExpose()
}

func (c *Config) getHeaders() string {
	if c.Cors.allheader || len(c.Cors.AllowedHeader) == 0 {
		return ""
	}
	removedup := make(map[string]struct{}, len(c.Cors.AllowedHeader))
	for _, v := range c.Cors.AllowedHeader {
		if v != "*" {
			removedup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
	}
	unique := make([]string, len(removedup))
	index := 0
	for v := range removedup {
		unique[index] = v
		index++
	}
	return strings.Join(unique, ", ")
}
func (c *Config) getExpose() string {
	if len(c.Cors.ExposeHeader) > 0 {
		removedup := make(map[string]struct{}, len(c.Cors.ExposeHeader))
		for _, v := range c.Cors.ExposeHeader {
			removedup[http.CanonicalHeaderKey(v)] = struct{}{}
		}
		unique := make([]string, len(removedup))
		index := 0
		for v := range removedup {
			unique[index] = v
			index++
		}
		return strings.Join(unique, ", ")
	} else {
		return ""
	}
}
