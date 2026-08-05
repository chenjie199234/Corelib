package mysql

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/chenjie199234/Corelib/internal/version"
	"github.com/chenjie199234/Corelib/util/ctime"

	gmysql "github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
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
	//false: Prepare has no effect.for every Query or Exec,client will assemble the sql and send once to get the result,server needs to analyse‌ every sql
	//  advantage: only need to send once,reduce the network cost
	//  disadvantage: server needs to analyse every sql,waste the server's cpu
	//true: every Query or Exec needs to Prepare sql first,then use the Prepare result(*stmt) to send params to get the Query or Exec result,finally needs to close the Prepare
	//  advantage: we can call Prepare alone,then keep the Prepare result(*stmt) and reuse it with different params,server only need to analyse sql once,this can reduce the server's cpu
	//  disadvantage: 1.if you use Query or Exec directly,there are three network cost,one for Prepare,one for send params,one for close,and server still needs to analyse every sql,waste server's cpu
	//                2.if you use Prepare alone to reuse the Prepare result(*stmt),the *stmt will occupy one connection until it be closed
	ServerSidePrepare bool `json:"server_side_prepare"`
	//0: default 100
	MaxOpen uint16 `json:"max_open"`
	//<=0: no idletime
	MaxConnIdletime ctime.Duration `json:"max_conn_idletime"`
	//<=0: default 5s
	DialTimeout ctime.Duration `json:"dial_timeout"`
	//<=0: no timeout,context's deadline > IOTimeout > context without deadline
	IOTimeout ctime.Duration `json:"io_timeout"`
}

type Client struct {
	mysqlname string
	master    Operator
	slave     Operator
}

// if tlsc is not nil,the tls will be activated
func NewMysql(c *Config, tlsc *tls.Config) (*Client, error) {
	if c.Master == nil || c.Master.Addr == "" {
		return nil, errors.New("missing master addr in config")
	}
	if c.Slaves != nil && len(c.Slaves.Addrs) > 1 {
		undup := make(map[string]struct{}, len(c.Slaves.Addrs))
		for _, addr := range c.Slaves.Addrs {
			undup[addr] = struct{}{}
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
	//driver's default(false) is UsePrepare(true)
	gmysqlc.InterpolateParams = !c.ServerSidePrepare

	client := &Client{
		mysqlname: c.MysqlName,
		master:    make([]*cdb, 0, 1),
		slave:     make([]*cdb, 0, 5),
	}
	lker := &sync.Mutex{}
	var e error
	wg := sync.WaitGroup{}
	wg.Go(func() {
		tmpc := gmysqlc.Clone()
		tmpc.Addr = c.Master.Addr
		if c.Master.UserName != "" {
			tmpc.User = c.Master.UserName
			tmpc.Passwd = c.Master.Password
			tmpc.AllowNativePasswords = true
		}
		connector, err := gmysql.NewConnector(tmpc)
		if err != nil {
			lker.Lock()
			e = err
			lker.Unlock()
			return
		}
		tmpdb := sql.OpenDB(connector)
		tmpdb.SetMaxOpenConns(int(c.MaxOpen))
		tmpdb.SetMaxIdleConns(int(c.MaxOpen))
		tmpdb.SetConnMaxIdleTime(c.MaxConnIdletime.StdDuration())
		lker.Lock()
		client.master = append(client.master, &cdb{db: tmpdb, master: true, addr: c.Master.Addr, name: c.MysqlName})
		lker.Unlock()
	})
	if c.Slaves != nil && len(c.Slaves.Addrs) > 0 {
		for _, v := range c.Slaves.Addrs {
			addr := v
			wg.Go(func() {
				tmpc := gmysqlc.Clone()
				tmpc.Addr = addr
				if c.Slaves.UserName != "" {
					tmpc.User = c.Slaves.UserName
					tmpc.Passwd = c.Slaves.Password
					tmpc.AllowNativePasswords = true
				}
				connector, err := gmysql.NewConnector(tmpc)
				if err != nil {
					lker.Lock()
					e = err
					lker.Unlock()
					return
				}
				tmpdb := sql.OpenDB(connector)
				tmpdb.SetMaxOpenConns(int(c.MaxOpen))
				tmpdb.SetMaxIdleConns(int(c.MaxOpen))
				tmpdb.SetConnMaxIdleTime(c.MaxConnIdletime.StdDuration())
				lker.Lock()
				client.slave = append(client.slave, &cdb{db: tmpdb, master: false, addr: addr, name: c.MysqlName})
				lker.Unlock()
			})
		}
	}
	wg.Wait()
	if e != nil {
		return nil, e
	}
	wg.Go(func() {
		if err := client.master.PingContext(context.Background()); err != nil {
			lker.Lock()
			e = err
			lker.Unlock()
		}
	})
	wg.Go(func() {
		if err := client.slave.PingContext(context.Background()); err != nil {
			lker.Lock()
			e = err
			lker.Unlock()
		}
	})
	wg.Wait()
	if e != nil {
		wg.Go(func() { client.master.Close() })
		wg.Go(func() { client.slave.Close() })
		wg.Wait()
	} else {
		tracer := otel.Tracer("Corelib.mysql.client", trace.WithInstrumentationVersion(version.String()))
		for _, v := range client.master {
			v.tracer = tracer
		}
		for _, v := range client.slave {
			v.tracer = tracer
		}
	}
	return client, e
}

func (c *Client) Master() Operator {
	if len(c.master) == 0 {
		panic("mysql:" + c.mysqlname + " doesn't exist master")
	}
	return c.master
}
func (c *Client) Slave() Operator {
	if len(c.slave) == 0 {
		panic("mysql:" + c.mysqlname + " doesn't exist slaves")
	}
	return c.slave
}
func (c *Client) PingContext(ctx context.Context) error {
	lker := sync.Mutex{}
	var e error
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		if err := c.master.PingContext(ctx); err != nil {
			lker.Lock()
			e = err
			lker.Unlock()
		}
	})
	wg.Go(func() {
		if err := c.slave.PingContext(ctx); err != nil {
			lker.Lock()
			e = err
			lker.Unlock()
		}
	})
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
