package redis

import (
	"context"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

type Pool struct {
	p *redis.Pool
}

type Conn struct {
	c redis.Conn
}

type Config struct {
	Username    string
	Password    string
	Addr        string
	MaxOpen     int
	MaxIdletime time.Duration
	IOTimeout   time.Duration
	ConnTimeout time.Duration
}

var ErrNil = redis.ErrNil
var ErrPoolExhausted = redis.ErrPoolExhausted

var p *sync.Pool

func init() {
	p = &sync.Pool{}
}
func NewRedis(c *Config) *Pool {
	return &Pool{
		p: &redis.Pool{
			DialContext: func(ctx context.Context) (redis.Conn, error) {
				if c.ConnTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, c.ConnTimeout)
					defer cancel()
				}
				conn, e := redis.DialContext(ctx, "tcp", c.Addr, redis.DialReadTimeout(c.IOTimeout), redis.DialWriteTimeout(c.IOTimeout))
				if e != nil {
					return nil, e
				}
				cc := &Conn{c: conn}
				if c.Username != "" && c.Password != "" {
					if _, e = cc.DoContext(ctx, "AUTH", c.Username, c.Password); e != nil {
						conn.Close()
						return nil, e
					}
				} else if c.Password != "" {
					if _, e = cc.DoContext(ctx, "AUTH", c.Password); e != nil {
						conn.Close()
						return nil, e
					}
				}
				return conn, nil
			},
			MaxActive:   c.MaxOpen,
			IdleTimeout: c.MaxIdletime,
		},
	}
}
func getconn(conn redis.Conn) *Conn {
	c, ok := p.Get().(*Conn)
	if !ok {
		return &Conn{c: conn}
	}
	c.c = conn
	return c
}
func putconn(c *Conn) {
	c.c.Close()
	p.Put(c)
}
func (p *Pool) GetRedis() *redis.Pool {
	return p.p
}
func (p *Pool) GetContext(ctx context.Context) (*Conn, error) {
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	return getconn(c), nil
}

func (c *Conn) DoContext(ctx context.Context, cmd string, args ...interface{}) (interface{}, error) {
	dl, ok := ctx.Deadline()
	if ok {
		timeout := time.Until(dl)
		if timeout <= 0 {
			return nil, context.DeadlineExceeded
		}
		return c.c.(redis.ConnWithTimeout).DoWithTimeout(timeout, cmd, args...)
	} else {
		return c.c.Do(cmd, args...)
	}
}
func (c *Conn) Send(cmd string, args ...interface{}) error {
	return c.c.Send(cmd, args...)
}
func (c *Conn) Flush() error {
	return c.c.Flush()
}
func (c *Conn) ReceiveContext(ctx context.Context) (interface{}, error) {
	dl, ok := ctx.Deadline()
	if ok {
		timeout := time.Until(dl)
		if timeout <= 0 {
			return nil, context.DeadlineExceeded
		}
		return c.c.(redis.ConnWithTimeout).ReceiveWithTimeout(timeout)
	} else {
		return c.c.Receive()
	}
}
func (c *Conn) Close() {
	putconn(c)
}
func Int(reply interface{}, e error) (int, error) {
	return redis.Int(reply, e)
}
func Ints(reply interface{}, e error) ([]int, error) {
	return redis.Ints(reply, e)
}
func IntMap(reply interface{}, e error) (map[string]int, error) {
	return redis.IntMap(reply, e)
}
func Int64(reply interface{}, e error) (int64, error) {
	return redis.Int64(reply, e)
}
func Int64s(reply interface{}, e error) ([]int64, error) {
	return redis.Int64s(reply, e)
}
func Int64Map(reply interface{}, e error) (map[string]int64, error) {
	return redis.Int64Map(reply, e)
}
func Uint64(reply interface{}, e error) (uint64, error) {
	return redis.Uint64(reply, e)
}
func Uint64s(reply interface{}, e error) ([]uint64, error) {
	return redis.Uint64s(reply, e)
}
func Uint64Map(reply interface{}, e error) (map[string]uint64, error) {
	return redis.Uint64Map(reply, e)
}
func Float64(reply interface{}, e error) (float64, error) {
	return redis.Float64(reply, e)
}
func Float64s(reply interface{}, e error) ([]float64, error) {
	return redis.Float64s(reply, e)
}
func String(reply interface{}, e error) (string, error) {
	return redis.String(reply, e)
}
func Strings(reply interface{}, e error) ([]string, error) {
	return redis.Strings(reply, e)
}
func StringMap(reply interface{}, e error) (map[string]string, error) {
	return redis.StringMap(reply, e)
}
func Bytes(reply interface{}, e error) ([]byte, error) {
	return redis.Bytes(reply, e)
}
func ByteSlices(reply interface{}, e error) ([][]byte, error) {
	return redis.ByteSlices(reply, e)
}
func Bool(reply interface{}, e error) (bool, error) {
	return redis.Bool(reply, e)
}
func Values(reply interface{}, e error) ([]interface{}, error) {
	return redis.Values(reply, e)
}
func Positions(reply interface{}, e error) ([]*[2]float64, error) {
	return redis.Positions(reply, e)
}
