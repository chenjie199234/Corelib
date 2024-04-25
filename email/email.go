package email

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"math"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenjie199234/Corelib/container/list"
	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/ctime"
)

type EmailClientConfig struct {
	EmailName       string         `json:"email_name"`
	MaxOpen         uint16         `json:"max_open"`          //0: default 100
	MaxConnIdletime ctime.Duration `json:"max_conn_idletime"` //<=0 means no idle time out
	Host            string         `json:"host"`
	Port            uint16         `json:"port"`
	Account         string         `json:"account"`
	Password        string         `json:"password"`
}
type EmailClient struct {
	c      *EmailClientConfig
	p      *list.List[*smtpclient]
	notice chan *struct{}
	lker   sync.Mutex
	count  uint32
}
type smtpclient struct {
	lker    sync.Mutex
	client  *smtp.Client
	tmer    *time.Timer
	expired bool
}

func (c *smtpclient) sendemail(from string, to []string, email []byte) (e error, del bool) {
	defer func() {
		if e != nil && c.client.Reset() != nil {
			del = true
		}
	}()
	if e = c.client.Mail(from); e != nil {
		return
	}
	for _, v := range to {
		if e = c.client.Rcpt(v); e != nil {
			return
		}
	}
	var w io.WriteCloser
	if w, e = c.client.Data(); e != nil {
		return
	}
	if _, e = w.Write(email); e != nil {
		return
	}
	e = w.Close()
	return
}

func NewEmailClient(c *EmailClientConfig) (*EmailClient, error) {
	if c.MaxOpen == 0 {
		c.MaxOpen = 100
	}
	if c.Host == "" || c.Port == 0 {
		return nil, errors.New("missing host/port in the config")
	}
	if c.Account == "" || c.Password == "" {
		return nil, errors.New("missing account/password in the config")
	}
	client := &EmailClient{c: c, p: list.NewList[*smtpclient](), notice: make(chan *struct{}, 1)}
	return client, nil
}
func (c *EmailClient) get(ctx context.Context) (*smtpclient, error) {
	for {
		cc, e := c.p.Pop(nil)
		if e == nil {
			if c.c.MaxConnIdletime <= 0 {
				select {
				case c.notice <- nil:
				default:
				}
				return cc, nil
			}
			cc.lker.Lock()
			if cc.expired || !cc.tmer.Stop() {
				cc.lker.Unlock()
				atomic.AddUint32(&c.count, math.MaxUint32)
				continue
			}
			cc.tmer = time.AfterFunc(c.c.MaxConnIdletime.StdDuration(), func() {
				cc.lker.Lock()
				defer cc.lker.Unlock()
				cc.tmer.Stop()
				cc.expired = true
				cc.client.Close()
			})
			cc.lker.Unlock()
			select {
			case c.notice <- nil:
			default:
			}
			return cc, nil
		}
		//pool is empty
		if c.c.MaxOpen == 0 {
			//no limit
			atomic.AddUint32(&c.count, 1)
			return c.new()
		}
		//limit check
		c.lker.Lock()
		if c.count < uint32(c.c.MaxOpen) {
			tmp, e := c.new()
			if e != nil {
				c.lker.Unlock()
				return nil, e
			}
			if atomic.AddUint32(&c.count, 1) < uint32(c.c.MaxOpen) {
				select {
				case c.notice <- nil:
				default:
				}
			}
			c.lker.Unlock()
			return tmp, nil
		}
		c.lker.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.notice:
		}
	}
}
func (c *EmailClient) put(client *smtpclient) {
	c.p.Push(client)
	select {
	case c.notice <- nil:
	default:
	}
}
func (c *EmailClient) new() (*smtpclient, error) {
	client, e := smtp.Dial(c.c.Host + ":" + strconv.FormatUint(uint64(c.c.Port), 10))
	if e != nil {
		return nil, e
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if e = client.StartTLS(&tls.Config{ServerName: c.c.Host}); e != nil {
			return nil, e
		}
	}
	if ok, _ := client.Extension("AUTH"); ok {
		if e = client.Auth(smtp.PlainAuth("", c.c.Account, c.c.Password, c.c.Host)); e != nil {
			return nil, e
		}
	}
	cc := &smtpclient{
		lker:    sync.Mutex{},
		client:  client,
		expired: false,
	}
	if c.c.MaxConnIdletime > 0 {
		cc.tmer = time.AfterFunc(c.c.MaxConnIdletime.StdDuration(), func() {
			cc.lker.Lock()
			defer cc.lker.Unlock()
			cc.tmer.Stop()
			cc.expired = true
			cc.client.Close()
		})
	}
	return cc, nil
}
func (c *EmailClient) SendTextEmail(ctx context.Context, to []string, subject string, body []byte) error {
	ctx, span := trace.NewSpan(ctx, "Corelib.Email", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("email", c.c.EmailName)
	span.GetSelfSpanData().SetStateKV("host", c.c.Host+":"+strconv.FormatUint(uint64(c.c.Port), 10))
	span.GetSelfSpanData().SetStateKV("subject", subject)
	span.GetSelfSpanData().SetStateKV("self", c.c.Account)
	count := 0
	for _, v := range to {
		count += len(v)
	}
	if count > 256 {
		span.GetSelfSpanData().SetStateKV("targets", strconv.Itoa(len(to))+" targets")
	} else {
		span.GetSelfSpanData().SetStateKV("targets", strings.Join(to, ";"))
	}
	e := c.sendemail(ctx, to, subject, "text/plain; charset=UTF-8", body)
	span.Finish(e)
	return e
}
func (c *EmailClient) SendHtmlEmail(ctx context.Context, to []string, subject string, body []byte) error {
	ctx, span := trace.NewSpan(ctx, "Corelib.Email", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("email", c.c.EmailName)
	span.GetSelfSpanData().SetStateKV("host", c.c.Host+":"+strconv.FormatUint(uint64(c.c.Port), 10))
	span.GetSelfSpanData().SetStateKV("subject", subject)
	span.GetSelfSpanData().SetStateKV("self", c.c.Account)
	count := 0
	for _, v := range to {
		count += len(v)
	}
	if count > 256 {
		span.GetSelfSpanData().SetStateKV("targets", strconv.Itoa(len(to))+" targets")
	} else {
		span.GetSelfSpanData().SetStateKV("targets", strings.Join(to, ";"))
	}
	e := c.sendemail(ctx, to, subject, "text/html; charset=UTF-8", body)
	span.Finish(e)
	return e
}
func (c *EmailClient) sendemail(ctx context.Context, to []string, subject string, mimetype string, body []byte) error {
	for {
		client, e := c.get(ctx)
		if e != nil {
			return e
		}
		client.lker.Lock()
		if client.expired {
			client.lker.Unlock()
			atomic.AddUint32(&c.count, math.MaxUint32)
			continue
		}
		if e := client.client.Noop(); e != nil {
			client.client.Close()
			if client.tmer != nil {
				client.tmer.Stop()
			}
			client.lker.Unlock()
			atomic.AddUint32(&c.count, math.MaxUint32)
			continue
		}
		email := c.formemail(to, subject, mimetype, body)
		if e, del := client.sendemail(c.c.Account, to, email); e != nil {
			if del {
				client.client.Close()
				if client.tmer != nil {
					client.tmer.Stop()
				}
				atomic.AddUint32(&c.count, math.MaxUint32)
				select {
				case c.notice <- nil:
				default:
				}
			}
			client.lker.Unlock()
			pool.GetPool().Put(&email)
			return e
		}
		client.lker.Unlock()
		c.put(client)
		pool.GetPool().Put(&email)
		return nil
	}
}
func (c *EmailClient) formemail(to []string, subject string, mimetype string, body []byte) []byte {
	//from
	count := 6 + len(c.c.Account) + 2
	//to
	count += 4
	for _, v := range to {
		count += len(v)
	}
	count += len(to) - 1
	count += 2
	//mimetype
	count += 14 + len(mimetype) + 2
	//subject
	count += 9 + len(subject) + 2
	//empty line
	count += 2
	//body
	count += len(body)

	buf := pool.GetPool().Get(count)
	buf = buf[:0]
	//from
	buf = append(buf, "From: "...)
	buf = append(buf, c.c.Account...)
	buf = append(buf, "\r\n"...)
	//to
	buf = append(buf, "To: "...)
	buf = append(buf, strings.Join(to, ",")...)
	buf = append(buf, "\r\n"...)
	//mimetype
	buf = append(buf, "Content-Type: "...)
	buf = append(buf, mimetype...)
	buf = append(buf, "\r\n"...)
	//subject
	buf = append(buf, "Subject: "...)
	buf = append(buf, subject...)
	buf = append(buf, "\r\n\r\n"...)
	buf = append(buf, body...)
	return buf
}
