package ws

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"net"
	"net/http"

	"github.com/chenjie199234/Corelib/pool"
	"github.com/chenjie199234/Corelib/util/common"
)

// doesn't support sub protocol and extension
// ver conn net.Conn
// ... get the server conn
// ... don't forget to set the timeout on the conn
// reader := bufio.NewReader(conn)
// Cupgrade(reader, conn)
func Cupgrade(reader *bufio.Reader, writer net.Conn, host, path string) (header http.Header, e error) {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)
	buf.AppendString("GET ")
	if path == "" {
		buf.AppendString("/ HTTP/1.1\r\n")
	} else if path[0] != '/' {
		buf.AppendByte('/')
		buf.AppendString(path)
		buf.AppendString(" HTTP/1.1\r\n")
	} else {
		buf.AppendString(path)
		buf.AppendString(" HTTP/1.1\r\n")
	}
	buf.AppendString("host: ")
	buf.AppendString(host)
	buf.AppendString("\r\n")
	buf.AppendString("connection: Upgrade\r\n")
	buf.AppendString("upgrade: websocket\r\n")
	buf.AppendString("sec-websocket-version: 13\r\n")
	buf.AppendString("sec-webSocket-key: ")
	nonce := make([]byte, 16)
	rand.Read(nonce)
	noncestr := base64.StdEncoding.EncodeToString(nonce)
	buf.AppendString(noncestr)
	buf.AppendString("\r\n")
	buf.AppendString("\r\n")
	if _, e = writer.Write(buf.Bytes()); e != nil {
		return
	}
	buf.Reset()
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
			buf.AppendByteSlice(line)
			continue
		}
		if len(line) == 0 && buf.Len() == 0 {
			//finish the header
			break
		}
		if buf.Len() == 0 {
			//deal the line
		} else if len(line) == 0 {
			//deal the buf
			line = buf.Bytes()
		} else {
			//deal the buf+line
			buf.AppendByteSlice(line)
			line = buf.Bytes()
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
			switch common.Byte2str(bytes.ToLower(pieces[0])) {
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
			case "sec-websocket-version":
				check |= 0b00000100
				if !bytes.Equal(pieces[1], []byte{'1', '3'}) {
					e = ErrHeaderLineFormat
					return
				}
			case "sec-websocket-accept":
				check |= 0b00001000
				h := sha1.New()
				h.Write(common.Str2byte(noncestr))
				h.Write([]byte{'2', '5', '8', 'E', 'A', 'F', 'A', '5', '-', 'E', '9', '1', '4', '-', '4', '7', 'D', 'A', '-', '9', '5', 'C', 'A', '-', 'C', '5', 'A', 'B', '0', 'D', 'C', '8', '5', 'B', '1', '1'})
				if base64.StdEncoding.EncodeToString(h.Sum(nil)) != common.Byte2str(pieces[1]) {
					e = ErrSign
					return
				}
			case "sec-websocket-protocol":
				//doesn't support
			case "sec-websocket-extensions":
				//doesn't support
			default:
				header.Add(string(pieces[0]), string(pieces[1]))
			}
		}
		buf.Reset()
	}
	if check != 0b00001111 {
		e = ErrHeaderLineFormat
		return
	}
	return
}
