package email

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/chenjie199234/Corelib/log/trace"
	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/pool/cpool"
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
	c *EmailClientConfig
	p *cpool.CPool[*smtp.Client]
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
	client := &EmailClient{
		c: c,
		p: cpool.NewCPool(uint32(c.MaxOpen), func() (*smtp.Client, error) {
			client, e := smtp.Dial(c.Host + ":" + strconv.FormatUint(uint64(c.Port), 10))
			if e != nil {
				return nil, e
			}
			if ok, _ := client.Extension("STARTTLS"); ok {
				if e = client.StartTLS(&tls.Config{ServerName: c.Host}); e != nil {
					return nil, e
				}
			}
			if ok, _ := client.Extension("AUTH"); ok {
				if e = client.Auth(smtp.PlainAuth("", c.Account, c.Password, c.Host)); e != nil {
					return nil, e
				}
			}
			return client, nil
		}, c.MaxConnIdletime.StdDuration(), func(client *smtp.Client) {
			client.Close()
		}),
	}
	return client, nil
}
func (c *EmailClient) SendTextEmail(ctx context.Context, to []string, subject string, body []byte) error {
	return c.do(ctx, to, subject, "text/plain; charset=UTF-8", body)
}
func (c *EmailClient) SendHtmlEmail(ctx context.Context, to []string, subject string, body []byte) error {
	return c.do(ctx, to, subject, "text/html; charset=UTF-8", body)
}
func (c *EmailClient) do(ctx context.Context, to []string, subject string, mimetype string, body []byte) (e error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Email", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("email", c.c.EmailName)
	span.GetSelfSpanData().SetStateKV("host", c.c.Host+":"+strconv.FormatUint(uint64(c.c.Port), 10))
	span.GetSelfSpanData().SetStateKV("subject", subject)
	span.GetSelfSpanData().SetStateKV("self", c.c.Account)
	count := 0
	for _, v := range to {
		count += len(v)
		if count > 256 {
			break
		}
	}
	if count > 256 {
		span.GetSelfSpanData().SetStateKV("targets", strconv.Itoa(len(to))+" targets")
	} else {
		span.GetSelfSpanData().SetStateKV("targets", strings.Join(to, ";"))
	}
	defer span.Finish(e)
	for {
		var client *smtp.Client
		if client, e = c.p.Get(ctx); e != nil {
			return
		}
		if e = client.Noop(); e != nil {
			client.Close()
			c.p.AbandonOne()
			continue
		}
		email := c.formemail(to, subject, mimetype, body)
		var del bool
		if e, del = c.sendemail(client, to, email); e != nil {
			if del {
				client.Close()
				c.p.AbandonOne()
			} else {
				c.p.Put(client)
			}
			bpool.Put(&email)
			return
		}
		c.p.Put(client)
		bpool.Put(&email)
		return
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

	buf := bpool.Get(count)
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
func (c *EmailClient) sendemail(client *smtp.Client, to []string, email []byte) (e error, del bool) {
	defer func() {
		if e != nil && client.Reset() != nil {
			del = true
		}
	}()
	if e = client.Mail(c.c.Account); e != nil {
		return
	}
	for _, v := range to {
		if e = client.Rcpt(v); e != nil {
			return
		}
	}
	var w io.WriteCloser
	if w, e = client.Data(); e != nil {
		return
	}
	if _, e = w.Write(email); e != nil {
		return
	}
	e = w.Close()
	return
}
