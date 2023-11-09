package ws

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"net"
	"net/http"

	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
)

// doesn't support sub protocol and extension
// example:
// ver conn net.Conn
// ... get the client conn
// ... don't forget to set the timeout on the conn
// reader := bufio.NewReader(conn)
// Supgrade(reader, conn)
func Supgrade(reader *bufio.Reader, writer net.Conn) (path string, header http.Header, e error) {
	peekdata, e := reader.Peek(1)
	if e != nil {
		return
	}
	if peekdata[0] != 'G' {
		e = ErrNotWS
		return
	}
	var accept string
	var check uint8
	header = make(http.Header)
	buf := pool.GetPool().Get(0)
	defer pool.GetPool().Put(&buf)
	for {
		line, prefix, err := reader.ReadLine()
		if e != nil {
			e = err
			return
		}
		if prefix {
			if len(buf)+len(line) <= cap(buf) {
				buf = append(buf, line...)
			} else {
				e = ErrHeaderLineFormat
				return
			}
			continue
		}
		if len(line) == 0 && len(buf) == 0 {
			//finish the header
			break
		}
		if len(buf) == 0 {
			//deal the line
		} else if len(line) == 0 {
			//deal the buf
			line = buf
		} else if len(buf)+len(line) <= cap(buf) {
			//deal the buf+line
			buf = append(buf, line...)
			line = buf
		} else {
			e = ErrHeaderLineFormat
			return
		}
		//deal
		if path == "" {
			//deal the request line
			pieces := bytes.Split(line, []byte{' '})
			for i := range pieces {
				pieces[i] = bytes.TrimSpace(pieces[i])
			}
			if len(pieces) != 3 {
				e = ErrRequestLineFormat
				return
			}
			if !bytes.Equal(pieces[0], []byte{'G', 'E', 'T'}) {
				e = ErrHttpMethod
				return
			}
			if !bytes.Equal(pieces[2], []byte{'H', 'T', 'T', 'P', '/', '1', '.', '1'}) {
				e = ErrHttpVersion
				return
			}
			if len(pieces[1]) == 0 {
				path = "/"
			} else if pieces[1][0] != '/' {
				path = "/" + string(pieces[1])
			} else {
				path = string(pieces[1])
			}
		} else {
			//deal the header line
			index := bytes.Index(line, []byte{':'})
			if index == -1 {
				e = ErrHeaderLineFormat
				return
			}
			pieces := [][]byte{bytes.TrimSpace(line[:index]), bytes.TrimSpace(line[index+1:])}
			switch common.BTS(bytes.ToLower(pieces[0])) {
			case "host":
				check |= 0b00000001
			case "connection":
				check |= 0b00000010
				if !bytes.Equal(bytes.ToLower(pieces[1]), []byte{'u', 'p', 'g', 'r', 'a', 'd', 'e'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "upgrade":
				check |= 0b00000100
				if !bytes.Equal(bytes.ToLower(pieces[1]), []byte{'w', 'e', 'b', 's', 'o', 'c', 'k', 'e', 't'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "sec-websocket-version":
				check |= 0b00001000
				if !bytes.Equal(pieces[1], []byte{'1', '3'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "sec-websocket-key":
				check |= 0b00010000
				h := sha1.New()
				h.Write(pieces[1])
				h.Write([]byte{'2', '5', '8', 'E', 'A', 'F', 'A', '5', '-', 'E', '9', '1', '4', '-', '4', '7', 'D', 'A', '-', '9', '5', 'C', 'A', '-', 'C', '5', 'A', 'B', '0', 'D', 'C', '8', '5', 'B', '1', '1'})
				accept = base64.StdEncoding.EncodeToString(h.Sum(nil))
			case "sec-websocket-protocol":
				//doesn't support
			case "sec-websocket-extensions":
				//doesn't support
			default:
				//other headers
				header.Add(string(pieces[0]), string(pieces[1]))
			}
		}
		buf = buf[:0]
	}
	if check != 0b00011111 {
		e = ErrHeaderLineFormat
		return
	}
	buf = append(buf, "HTTP/1.1 101 Switching Protocols\r\n"...)
	buf = append(buf, "Upgrade: websocket\r\n"...)
	buf = append(buf, "Connection: Upgrade\r\n"...)
	buf = append(buf, "Sec-WebSocket-Version: 13\r\n"...)
	buf = append(buf, "Sec-WebSocket-Accept: "...)
	buf = append(buf, accept...)
	buf = append(buf, "\r\n\r\n"...)
	if _, e = writer.Write(buf); e != nil {
		return
	}
	return
}
