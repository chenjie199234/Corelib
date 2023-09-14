package mysql

import (
	"context"
	"crypto/tls"
	"database/sql"
	"time"

	gmysql "github.com/go-sql-driver/mysql"
)

type Config struct {
	//the mysql instance's name
	MysqlName string `json:"mysql_name"`
	//only support tcp socket
	//ip:port or host:port
	Addr     string `json:"addr"`
	UserName string `json:"user_name"`
	Password string `json:"password"`
	//default utf8mb4
	Charset string `json:"charset"`
	//default utf8mb4_general_ci
	Collation string `json:"collation"`
	ParseTime bool   `json:"parse_time"`
	//0: default 100
	MaxOpen uint16 `json:"max_open"`
	//<=0: no idletime
	MaxConnIdletime time.Duration `json:"max_conn_idletime"`
	//<=0: default 5s
	DialTimeout time.Duration `json:"dial_timeout"`
	//<=0: no timeout
	IOTimeout time.Duration `json:"io_timeout"`
}
type Client struct {
	*sql.DB
}

// if tlsc is not nil,the tls will be actived
func NewMysql(c *Config, tlsc *tls.Config) (*Client, error) {
	var gmysqlc *gmysql.Config
	gmysqlc = gmysql.NewConfig()
	gmysqlc.Net = "tcp"
	gmysqlc.Addr = c.Addr
	if c.UserName != "" && c.Password != "" {
		gmysqlc.User = c.UserName
		gmysqlc.Passwd = c.Password
		gmysqlc.AllowNativePasswords = true
	}
	if c.Charset != "" {
		gmysqlc.Params = map[string]string{"charset": c.Charset}
	} else {
		gmysqlc.Params = map[string]string{"charset": "utf8mb4"}
	}
	if c.Collation != "" {
		gmysqlc.Collation = c.Collation
	} else {
		gmysqlc.Collation = "utf8mb4_general_ci"
	}
	gmysqlc.TLS = tlsc
	if c.DialTimeout <= 0 {
		gmysqlc.Timeout = time.Second * 5
	} else {
		gmysqlc.Timeout = c.DialTimeout
	}
	if c.IOTimeout > 0 {
		gmysqlc.ReadTimeout = c.IOTimeout
		gmysqlc.WriteTimeout = c.IOTimeout
	}
	gmysqlc.CheckConnLiveness = true
	gmysqlc.ParseTime = c.ParseTime
	connector, e := gmysql.NewConnector(gmysqlc)
	if e != nil {
		return nil, e
	}
	db := sql.OpenDB(connector)
	if c.MaxOpen == 0 {
		db.SetMaxOpenConns(100)
	} else {
		db.SetMaxOpenConns(int(c.MaxOpen))
	}
	db.SetConnMaxIdleTime(c.MaxConnIdletime)
	if e = db.PingContext(context.Background()); e != nil {
		return nil, e
	}
	//TODO add otel
	return &Client{db}, nil
}
