package redis

import (
	"context"
	"time"

	// "github.com/chenjie199234/Corelib/trace"

	"github.com/gomodule/redigo/redis"
)

type Pool struct {
	c *Config
	p *redis.Pool
}

type Conn struct {
	c   redis.Conn
	ctx context.Context //this can be used with trace log
}

type Config struct {
	RedisName   string
	URL         string        //[redis/rediss]://[username:]password@host/0
	MaxOpen     int           //this will overwrite the param in url
	MaxIdletime time.Duration //this will overwrite the param in url
	ConnTimeout time.Duration //this will overwrite the param in url
	IOTimeout   time.Duration //this will overwrite the param in url
}

var ErrNil = redis.ErrNil
var ErrPoolExhausted = redis.ErrPoolExhausted

func NewRedis(c *Config) *Pool {
	return &Pool{
		c: &Config{
			RedisName:   c.RedisName,
			URL:         c.URL,
			MaxOpen:     c.MaxOpen,
			MaxIdletime: c.MaxIdletime,
			ConnTimeout: c.ConnTimeout,
			IOTimeout:   c.IOTimeout,
		},
		p: &redis.Pool{
			DialContext: func(ctx context.Context) (redis.Conn, error) {
				conn, e := redis.DialURL(c.URL, redis.DialConnectTimeout(c.ConnTimeout), redis.DialReadTimeout(c.IOTimeout), redis.DialWriteTimeout(c.IOTimeout))
				if e != nil {
					return nil, e
				}
				return conn, nil
			},
			MaxIdle:     c.MaxOpen,
			MaxActive:   c.MaxOpen,
			IdleTimeout: c.MaxIdletime,
		},
	}
}

func (p *Pool) GetContext(ctx context.Context) (*Conn, error) {
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	return &Conn{ctx: ctx, c: c}, nil
}
func (p *Pool) Ping(ctx context.Context) error {
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return e
	}
	_, e = c.(redis.ConnWithContext).DoContext(ctx, "PING")
	return e
}

func (c *Conn) DoContext(ctx context.Context, cmd string, args ...interface{}) (interface{}, error) {
	return c.c.(redis.ConnWithContext).DoContext(ctx, cmd, args...)
}
func (c *Conn) Send(ctx context.Context, cmd string, args ...interface{}) error {
	return c.c.Send(cmd, args...)
}
func (c *Conn) Flush(ctx context.Context) error {
	return c.c.Flush()
}
func (c *Conn) ReceiveContext(ctx context.Context) (interface{}, error) {
	return c.c.(redis.ConnWithContext).ReceiveContext(ctx)
}
func (c *Conn) Err() error {
	return c.c.Err()
}
func (c *Conn) Close() {
	if c == nil {
		return
	}
	c.c.Close()
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
