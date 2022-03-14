package stream

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"math/rand"
	"net/url"
	"strconv"

	"github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
)

var wsSeckey = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

func (this *Instance) wsServerHandshake(p *Peer) bool {
	//websocket handshake
	method, _, protocol, e := wsFirstLine(p)
	if e != nil {
		log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error:", e)
		return false
	}
	if !bytes.Equal(method, []byte{'G', 'E', 'T'}) {
		log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: http method not GET")
		return false
	}
	if !bytes.Equal(protocol, []byte{'H', 'T', 'T', 'P', '/', '1', '.', '1'}) {
		log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: http version not HTTP/1.1")
		return false
	}
	var check uint8
	var accept string
	//head line
	for {
		header, headerdata, e := wsHeadLine(p)
		if e != nil {
			log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error:", e)
			return false
		} else if len(header) == 0 && len(headerdata) == 0 {
			break
		}
		switch common.Byte2str(header) {
		case "Host":
			check |= 0b00000001
		case "Connection":
			check |= 0b00000010
			if !bytes.Equal(headerdata, []byte{'U', 'p', 'g', 'r', 'a', 'd', 'e'}) {
				log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: header Connection format wrong")
				return false
			}
		case "Upgrade":
			check |= 0b00000100
			if !bytes.Equal(headerdata, []byte{'w', 'e', 'b', 's', 'o', 'c', 'k', 'e', 't'}) {
				log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: header Upgrade format wrong")
				return false
			}
		case "Sec-WebSocket-Version":
			check |= 0b00001000
			if !bytes.Equal(headerdata, []byte{'1', '3'}) {
				log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: header Sec-WebSocket-Version format wrong")
				return false
			}
		case "Sec-WebSocket-Key":
			check |= 0b00010000
			encoder := sha1.New()
			encoder.Write(headerdata)
			encoder.Write(wsSeckey)
			accept = base64.StdEncoding.EncodeToString(encoder.Sum(nil))
		case "X-Forwarded-For":
			if len(headerdata) > 0 {
				if ip := string(bytes.TrimSpace(bytes.Split(headerdata, []byte{','})[0])); ip != "" {
					p.realPeerIP = ip
				}
			}
		case "X-Real-Ip":
			if len(headerdata) > 0 {
				p.realPeerIP = string(headerdata)
			}
		}
	}
	if check != 0b00011111 {
		log.Error(nil, "[Stream.wsServerHandshake] from:", p.c.RemoteAddr().String(), "error: missing head")
		return false
	}
	//write server handshake
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	buf.AppendString("HTTP/1.1 101 Switching Protocols\r\n")
	buf.AppendString("Upgrade: websocket\r\n")
	buf.AppendString("Connection: Upgrade\r\n")
	buf.AppendString("Sec-WebSocket-Accept: ")
	buf.AppendString(accept)
	buf.AppendString("\r\n")
	buf.AppendString("\r\n")
	if _, e := p.c.Write(buf.Bytes()); e != nil {
		log.Error(nil, "[Stream.wsServerHandshake] to:", p.c.RemoteAddr().String(), "error:", e)
		return false
	}
	return true
}

func wsClientHandshake(p *Peer, u *url.URL) bool {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	buf.AppendString("GET ")
	if u.Path == "" {
		buf.AppendString("/ HTTP/1.1\r\n")
	} else {
		if u.Path[0] != '/' {
			buf.AppendByte('/')
		}
		buf.AppendString(u.Path)
		buf.AppendByte(' ')
		buf.AppendString("HTTP/1.1\r\n")
	}
	buf.AppendString("Host: ")
	buf.AppendString(u.Host)
	buf.AppendString("\r\n")
	buf.AppendString("Connection: Upgrade\r\n")
	buf.AppendString("Upgrade: websocket\r\n")
	buf.AppendString("Sec-WebSocket-Version: 13\r\n")
	buf.AppendString("Sec-WebSocket-Key: ")
	m := md5.Sum(common.Str2byte(strconv.Itoa(rand.Int())))
	b64m := base64.StdEncoding.EncodeToString(m[:])
	buf.AppendString(b64m)
	buf.AppendString("\r\n")
	buf.AppendString("\r\n")
	if _, e := p.c.Write(buf.Bytes()); e != nil {
		log.Error(nil, "[Stream.wsClientHandshake] to:", p.c.RemoteAddr().String(), "error:", e)
		return false
	}
	protocol, statuscode, statustext, e := wsFirstLine(p)
	if e != nil {
		log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error:", e)
		return false
	}
	if !bytes.Equal(statuscode, []byte{'1', '0', '1'}) {
		log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: status code:", statuscode, "status msg:", statustext)
		return false
	}
	if !bytes.Equal(protocol, []byte{'H', 'T', 'T', 'P', '/', '1', '.', '1'}) {
		log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: http version not HTTP/1.1")
		return false
	}
	var contentlen int
	for {
		//head line
		header, headerdata, e := wsHeadLine(p)
		if e != nil {
			log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error:", e)
			return false
		} else if len(header) == 0 && len(headerdata) == 0 {
			break
		}
		switch common.Byte2str(header) {
		case "Connection":
			if !bytes.Equal(headerdata, []byte{'U', 'p', 'g', 'r', 'a', 'd', 'e'}) {
				log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: header Connection format wrong")
				return false
			}
		case "Upgrade":
			if !bytes.Equal(headerdata, []byte{'w', 'e', 'b', 's', 'o', 'c', 'k', 'e', 't'}) {
				log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: header Upgrade format wrong")
				return false
			}
		case "Sec-WebSocket-Accept":
			encoder := sha1.New()
			encoder.Write(common.Str2byte(b64m))
			encoder.Write(wsSeckey)
			if base64.StdEncoding.EncodeToString(encoder.Sum(nil)) != common.Byte2str(headerdata) {
				log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: sec key verify failed")
				return false
			}
		case "Content-Length":
			if contentlen, e = strconv.Atoi(common.Byte2str(headerdata)); e != nil {
				log.Error(nil, "[Stream.wsClientHandshake] from:", p.c.RemoteAddr().String(), "error: header Content-Length format wrong")
				return false
			}
		}
	}
	//drop the body
	if _, e = p.cr.Discard(contentlen); e != nil {
		log.Error(nil, "[Stream.wsClientHandshakeww] from:", p.c.RemoteAddr().String(), "drop body error:", e)
		return false
	}
	return true
}

func wsFirstLine(p *Peer) (b1 []byte, b2 []byte, b3 []byte, e error) {
	line, e := wsHandshakeLine(p)
	if e != nil {
		return
	}
	line = bytes.TrimSpace(line)
	i := bytes.Index(line, []byte{' '})
	if i == -1 {
		b1 = line
		return
	}
	b1 = line[:i]
	line = bytes.TrimSpace(line[i+1:])
	i = bytes.Index(line, []byte{' '})
	if i == -1 {
		b2 = line
		return
	}
	b2 = line[:i]
	b3 = bytes.TrimSpace(line[i+1:])
	return
}
func wsHeadLine(p *Peer) (b1 []byte, b2 []byte, e error) {
	line, e := wsHandshakeLine(p)
	if e != nil {
		return
	}
	line = bytes.TrimSpace(line)
	i := bytes.Index(line, []byte{':'})
	if i == -1 {
		b1 = line
		return
	}
	b1 = line[:i]
	b2 = bytes.TrimSpace(line[i+1:])
	return
}

var errHttpLineLong = errors.New("http line too long")
var errHttpLineFormat = errors.New("http line format wrong")

func wsHandshakeLine(p *Peer) ([]byte, error) {
	line, e := p.cr.ReadSlice('\n')
	if e != nil {
		if e == bufio.ErrBufferFull {
			return nil, errHttpLineLong
		}
		return nil, e
	}
	//must end with \r\n
	if len(line) == 1 || line[len(line)-2] != '\r' {
		return nil, errHttpLineFormat
	}
	//drop \r\n
	return line[:len(line)-2], nil
}
