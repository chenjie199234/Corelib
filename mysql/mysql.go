package mysql

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/util/ctime"

	gmysql "github.com/go-sql-driver/mysql"
)

type Config struct {
	//the mysql instance's name
	MysqlName string `json:"mysql_name"`
	Master    *struct {
		//only support tcp socket,ip:port or host:port
		Addr     string `json:"addr"`
		UserName string `json:"user_name"`
		Password string `json:"password"`
	} `json:"master"`
	Slaves *struct {
		//only support tcp socket,ip:port or host:port
		Addrs []string `json:"addrs"`
		//to prevent misuse,the slave's user must be readonly
		UserName string `json:"user_name"`
		Password string `json:"password"`
	} `json:"slaves"`
	//default utf8mb4
	Charset string `json:"charset"`
	//default utf8mb4_unicode_ci
	Collation string `json:"collation"`
	ParseTime bool   `json:"parse_time"`
	//0: default 100
	MaxOpen uint16 `json:"max_open"`
	//<=0: no idletime
	MaxConnIdletime ctime.Duration `json:"max_conn_idletime"`
	//<=0: default 5s
	DialTimeout ctime.Duration `json:"dial_timeout"`
	//<=0: no timeout
	IOTimeout ctime.Duration `json:"io_timeout"`
}

type Client struct {
	mysqlname string
	master    Operator
	slave     Operator
}

// if tlsc is not nil,the tls will be actived
func NewMysql(c *Config, tlsc *tls.Config) (*Client, error) {
	if c.Master == nil || c.Master.Addr == "" {
		return nil, errors.New("missing master addr in config")
	}
	if c.Slaves != nil && len(c.Slaves.Addrs) > 1 {
		undup := make(map[string]*struct{}, len(c.Slaves.Addrs))
		for _, addr := range c.Slaves.Addrs {
			undup[addr] = nil
		}
		tmp := make([]string, 0, len(undup))
		for addr := range undup {
			tmp = append(tmp, addr)
		}
		c.Slaves.Addrs = tmp
	}
	if c.MaxOpen == 0 {
		c.MaxOpen = 100
	}
	var gmysqlc *gmysql.Config
	gmysqlc = gmysql.NewConfig()
	gmysqlc.Net = "tcp"
	defaultcharset := "utf8mb4"
	defaultcollation := "utf8mb4_unicode_ci"
	if c.Charset != "" {
		defaultcharset = c.Charset
	}
	if c.Collation != "" {
		defaultcollation = c.Collation
	}
	gmysqlc.Apply(gmysql.Charset(defaultcharset, defaultcollation))
	gmysqlc.TLS = tlsc
	if c.DialTimeout <= 0 {
		gmysqlc.Timeout = time.Second * 5
	} else {
		gmysqlc.Timeout = c.DialTimeout.StdDuration()
	}
	if c.IOTimeout > 0 {
		gmysqlc.ReadTimeout = c.IOTimeout.StdDuration()
		gmysqlc.WriteTimeout = c.IOTimeout.StdDuration()
	}
	gmysqlc.CheckConnLiveness = true
	gmysqlc.ParseTime = c.ParseTime

	var e error
	client := &Client{
		mysqlname: c.MysqlName,
		master:    make([]*cdb, 0, 1),
		slave:     make([]*cdb, 0, 5),
	}
	lker := &sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tmpc := gmysqlc.Clone()
		tmpc.Addr = c.Master.Addr
		if c.Master.UserName != "" {
			tmpc.User = c.Master.UserName
			tmpc.Passwd = c.Master.Password
			tmpc.AllowNativePasswords = true
		}
		connector, err := gmysql.NewConnector(tmpc)
		if err != nil {
			e = err
			return
		}
		tmpdb := sql.OpenDB(connector)
		tmpdb.SetMaxOpenConns(int(c.MaxOpen))
		tmpdb.SetConnMaxIdleTime(c.MaxConnIdletime.StdDuration())
		lker.Lock()
		client.master = append(client.master, &cdb{db: tmpdb, master: true, addr: c.Master.Addr, name: c.MysqlName})
		lker.Unlock()
	}()
	if c.Slaves != nil && len(c.Slaves.Addrs) > 0 {
		for _, v := range c.Slaves.Addrs {
			addr := v
			wg.Add(1)
			go func() {
				defer wg.Done()
				tmpc := gmysqlc.Clone()
				tmpc.Addr = addr
				if c.Slaves.UserName != "" {
					tmpc.User = c.Slaves.UserName
					tmpc.Passwd = c.Slaves.Password
					tmpc.AllowNativePasswords = true
				}
				connector, err := gmysql.NewConnector(tmpc)
				if err != nil {
					e = err
					return
				}
				tmpdb := sql.OpenDB(connector)
				tmpdb.SetMaxOpenConns(int(c.MaxOpen))
				tmpdb.SetConnMaxIdleTime(c.MaxConnIdletime.StdDuration())
				lker.Lock()
				client.slave = append(client.slave, &cdb{db: tmpdb, master: false, addr: addr, name: c.MysqlName})
				lker.Unlock()
			}()
		}
	}
	wg.Wait()
	if e != nil {
		return nil, e
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := client.master.PingContext(nil); err != nil {
			e = err
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := client.slave.PingContext(nil); err != nil {
			e = err
		}
	}()
	wg.Wait()
	return client, e
}

func (c *Client) Master() Operator {
	return c.master
}
func (c *Client) Slave() Operator {
	if len(c.slave) == 0 {
		panic("mysql:" + c.mysqlname + " doesn't exist slaves")
	}
	return c.slave
}
func (c *Client) PingContext(ctx context.Context) error {
	var e error
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := c.master.PingContext(ctx); err != nil {
			e = err
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := c.slave.PingContext(ctx); err != nil {
			e = err
		}
	}()
	wg.Wait()
	return e
}
func (c *Client) Close() error {
	if e := c.master.Close(); e != nil {
		return e
	}
	if e := c.slave.Close(); e != nil {
		return e
	}
	return nil
}
