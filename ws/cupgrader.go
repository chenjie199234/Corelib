package ws

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"net"
	"net/http"

	"github.com/chenjie199234/Corelib/pool/bpool"
	"github.com/chenjie199234/Corelib/util/common"
)

// doesn't support sub protocol and extension
// ver conn net.Conn
// ... get the server conn
// ... don't forget to set the timeout on the conn
// reader := bufio.NewReader(conn)
// Cupgrade(reader, conn)
func Cupgrade(reader *bufio.Reader, writer net.Conn, host, path string) (header http.Header, e error) {
	buf := bpool.Get(150 + len(host) + len(path))
	defer bpool.Put(&buf)
	buf = append(buf, "GET "...)
	if path == "" {
		buf = append(buf, "/ HTTP/1.1\r\n"...)
	} else if path[0] != '/' {
		buf = append(buf, '/')
		buf = append(buf, path...)
		buf = append(buf, " HTTP/1.1\r\n"...)
	} else {
		buf = append(buf, path...)
		buf = append(buf, " HTTP/1.1\r\n"...)
	}
	buf = append(buf, "host: "...)
	buf = append(buf, host...)
	buf = append(buf, "\r\n"...)
	buf = append(buf, "connection: Upgrade\r\n"...)
	buf = append(buf, "upgrade: websocket\r\n"...)
	buf = append(buf, "sec-websocket-version: 13\r\n"...)
	buf = append(buf, "sec-webSocket-key: "...)
	curlen := len(buf)
	buf = buf[:curlen+base64.StdEncoding.EncodedLen(16)]
	rand.Read(buf[curlen+base64.StdEncoding.EncodedLen(16)-16:])
	base64.StdEncoding.Encode(buf[curlen:], buf[curlen+base64.StdEncoding.EncodedLen(16)-16:])
	nonce := make([]byte, base64.StdEncoding.EncodedLen(16))
	copy(nonce, buf[curlen:]) //copy the base64 nonce
	buf = append(buf, "\r\n"...)
	buf = append(buf, "\r\n"...)
	if _, e = writer.Write(buf); e != nil {
		return
	}
	buf = buf[:0]
	var check uint8
	first := true
	header = make(http.Header)
	for {
		line, prefix, err := reader.ReadLine()
		if err != nil {
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
		if first {
			first = false
			//deal the response line
			pieces := bytes.Split(line, []byte{' '})
			for i := range pieces {
				pieces[i] = bytes.TrimSpace(pieces[i])
			}
			if len(pieces) != 4 {
				e = ErrResponseLineFormat
				return
			}
			if !bytes.Equal(pieces[0], []byte{'H', 'T', 'T', 'P', '/', '1', '.', '1'}) {
				e = ErrHttpVersion
				return
			}
			if !bytes.Equal(pieces[1], []byte{'1', '0', '1'}) {
				e = ErrResponseLineFormat
				return
			}
			if !bytes.Equal(pieces[2], []byte{'S', 'w', 'i', 't', 'c', 'h', 'i', 'n', 'g'}) || !bytes.Equal(pieces[3], []byte{'P', 'r', 'o', 't', 'o', 'c', 'o', 'l', 's'}) {
				e = ErrResponseLineFormat
				return
			}
		} else {
			//deal the header line
			index := bytes.Index(line, []byte{':'})
			if index == -1 {
				e = ErrHeaderLineFormat
				return
			}
			pieces := [][]byte{bytes.TrimSpace(line[:index]), bytes.TrimSpace(line[index+1:])}
			if len(pieces) != 2 {
				e = ErrHeaderLineFormat
				return
			}
			switch common.BTS(bytes.ToLower(pieces[0])) {
			case "connection":
				check |= 0b00000001
				if !bytes.Equal(bytes.ToLower(pieces[1]), []byte{'u', 'p', 'g', 'r', 'a', 'd', 'e'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "upgrade":
				check |= 0b00000010
				if !bytes.Equal(bytes.ToLower(pieces[1]), []byte{'w', 'e', 'b', 's', 'o', 'c', 'k', 'e', 't'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "sec-websocket-accept":
				check |= 0b00000100
				h := sha1.New()
				h.Write(nonce)
				h.Write([]byte{'2', '5', '8', 'E', 'A', 'F', 'A', '5', '-', 'E', '9', '1', '4', '-', '4', '7', 'D', 'A', '-', '9', '5', 'C', 'A', '-', 'C', '5', 'A', 'B', '0', 'D', 'C', '8', '5', 'B', '1', '1'})
				if base64.StdEncoding.EncodeToString(h.Sum(nil)) != common.BTS(pieces[1]) {
					e = ErrSign
					return
				}
			case "sec-websocket-version":
			case "sec-websocket-protocol":
				//doesn't support
			case "sec-websocket-extensions":
				//doesn't support
			default:
				header.Add(string(pieces[0]), string(pieces[1]))
			}
		}
		buf = buf[:0]
	}
	if check != 0b00000111 {
		e = ErrHeaderLineFormat
		return
	}
	return
}
