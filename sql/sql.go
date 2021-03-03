package sql

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
)

var ErrNoRows = sql.ErrNoRows
var ErrTxDone = sql.ErrTxDone
var ErrConnDone = sql.ErrConnDone

type Pool struct {
	p *sql.DB
}

type Config struct {
	Username    string
	Password    string
	Addr        string
	Collation   string
	MaxOpen     int
	MaxIdletime time.Duration
	IOTimeout   time.Duration
	ConnTimeout time.Duration
}

func NewMysql(c *Config) *Pool {
	db, _ := sql.Open("mysql", (&mysql.Config{
		User:                 c.Username,
		Passwd:               c.Password,
		Net:                  "tcp",
		Addr:                 c.Addr,
		Timeout:              c.ConnTimeout,
		WriteTimeout:         c.IOTimeout,
		ReadTimeout:          c.IOTimeout,
		AllowNativePasswords: true,
		Collation:            c.Collation,
	}).FormatDSN())
	db.SetMaxOpenConns(c.MaxOpen)
	db.SetConnMaxIdleTime(c.MaxIdletime)
	return &Pool{p: db}
}
func (p *Pool) Conn(ctx context.Context) (*sql.Conn, error) {
	return p.p.Conn(ctx)
}
func (p *Pool) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return p.p.BeginTx(ctx, nil)
}
