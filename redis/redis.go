package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"time"

	"github.com/chenjie199234/Corelib/trace"

	"github.com/gomodule/redigo/redis"
)

type Pool struct {
	c *Config
	p *redis.Pool
}

type Conn struct {
	p     *Pool
	c     redis.Conn
	start *time.Time
	ctx   context.Context
}

type Config struct {
	RedisName     string
	Username      string
	Password      string
	Addr          string
	MaxOpen       int
	MaxIdletime   time.Duration
	IOTimeout     time.Duration
	ConnTimeout   time.Duration
	UseTLS        bool
	SkipVerifyTLS bool     //don't verify the server's cert
	CAs           []string //CAs' path,specific the CAs need to be used,this will overwrite the default behavior:use the system's certpool
}

var ErrNil = redis.ErrNil
var ErrPoolExhausted = redis.ErrPoolExhausted

func NewRedis(c *Config) (*Pool, error) {
	options := make([]redis.DialOption, 0, 10)
	options = append(options, redis.DialReadTimeout(c.IOTimeout))
	options = append(options, redis.DialWriteTimeout(c.IOTimeout))
	options = append(options, redis.DialUsername(c.Username))
	options = append(options, redis.DialPassword(c.Password))
	if c.UseTLS {
		options = append(options, redis.DialUseTLS(true))
		var certpool *x509.CertPool
		if len(c.CAs) != 0 {
			certpool = x509.NewCertPool()
			for _, cert := range c.CAs {
				certPEM, e := os.ReadFile(cert)
				if e != nil {
					return nil, errors.New("[redis] read cert file:" + cert + " error:" + e.Error())
				}
				if !certpool.AppendCertsFromPEM(certPEM) {
					return nil, errors.New("[redis] load cert file:" + cert + " error:" + e.Error())
				}
			}
		}
		if c.SkipVerifyTLS || certpool != nil {
			options = append(options, redis.DialTLSConfig(&tls.Config{
				InsecureSkipVerify: c.SkipVerifyTLS,
				RootCAs:            certpool,
			}))
		}
	}
	return &Pool{
		c: c,
		p: &redis.Pool{
			DialContext: func(ctx context.Context) (redis.Conn, error) {
				if c.ConnTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, c.ConnTimeout)
					defer cancel()
				}
				conn, e := redis.DialContext(ctx, "tcp", c.Addr, options...)
				if e != nil {
					return nil, e
				}
				return conn, nil
			},
			MaxIdle:     c.MaxOpen,
			MaxActive:   c.MaxOpen,
			IdleTimeout: c.MaxIdletime,
		},
	}, nil
}

func (p *Pool) GetContext(ctx context.Context) (*Conn, error) {
	c, e := p.p.GetContext(ctx)
	if e != nil {
		return nil, e
	}
	//traceend := trace.TraceStart(ctx, trace.CLIENT, p.c.RedisName, p.c.Addr, "REDIS", "connect")
	start := time.Now()
	return &Conn{p: p, c: c, start: &start, ctx: ctx}, nil
}
func (p *Pool) Ping(ctx context.Context) error {
	c, e := p.GetContext(ctx)
	if e != nil {
		return e
	}
	_, e = c.DoContext(ctx, "PING")
	return e
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
func (c *Conn) Send(ctx context.Context, cmd string, args ...interface{}) error {
	return c.c.Send(cmd, args...)
}
func (c *Conn) Flush(ctx context.Context) error {
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
func (c *Conn) Err() error {
	return c.c.Err()
}
func (c *Conn) Close() {
	end := time.Now()
	trace.Trace(c.ctx, trace.CLIENT, c.p.c.RedisName, c.p.c.Addr, "REDIS", "connect", c.start, &end, c.c.Err())
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
