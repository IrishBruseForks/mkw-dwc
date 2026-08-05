// Package httpfix softens Go's strict HTTP parsing for Wii / Dolphin clients.
package httpfix

import (
	"bufio"
	"bytes"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// WrapListener returns a listener that strips duplicate Host headers before
// net/http sees the request. Mario Kart Wii (and Dolphin) send Host twice;
// Go rejects that with 400, which surfaces in-game as error 23400.
//
// component is the logging component name (for example "nas" or "proxy").
// When [Logging] DumpFile is set, raw recv/send bytes are written to that file.
//
// Keep-alive is supported: every request header block is rewritten, not only
// the first on the connection.
func WrapListener(l net.Listener, component string) net.Listener {
	if component == "" {
		component = "http"
	}
	return &listener{Listener: l, component: component}
}

type listener struct {
	net.Listener
	component string
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	dumpAccept(l.component, c.RemoteAddr())
	return &conn{
		Conn:      c,
		br:        bufio.NewReader(c),
		component: l.component,
		needHdr:   true,
	}, nil
}

type conn struct {
	net.Conn
	br        *bufio.Reader
	component string
	onceTLS   sync.Once
	tls       bool

	needHdr  bool   // next bytes should be an HTTP header block
	buf      []byte // pending bytes for net/http (rewritten headers)
	off      int
	bodyLeft int64 // remaining request body bytes
}

func (c *conn) Read(p []byte) (int, error) {
	c.onceTLS.Do(c.checkTLS)
	if c.tls {
		if c.off < len(c.buf) {
			n := copy(p, c.buf[c.off:])
			c.off += n
			return n, nil
		}
		return c.br.Read(p)
	}

	for {
		if c.off < len(c.buf) {
			n := copy(p, c.buf[c.off:])
			c.off += n
			return n, nil
		}
		c.buf = nil
		c.off = 0

		if c.bodyLeft > 0 {
			toRead := p
			if int64(len(toRead)) > c.bodyLeft {
				toRead = p[:c.bodyLeft]
			}
			n, err := c.br.Read(toRead)
			if n > 0 {
				c.bodyLeft -= int64(n)
				dumpDir(c.component, "recv", c.RemoteAddr(), toRead[:n])
				if c.bodyLeft == 0 {
					c.needHdr = true
				}
			}
			return n, err
		}

		// net/http idle-waits with Peek(4). Wait until at least one byte is
		// available before trying to consume a full header block, and do not
		// flip needHdr on a wait error (timeout / EOF).
		if _, err := c.br.Peek(1); err != nil {
			return 0, err
		}
		if err := c.loadHeaders(); err != nil {
			return 0, err
		}
	}
}

func (c *conn) Write(p []byte) (int, error) {
	if len(p) > 0 {
		dumpDir(c.component, "send", c.RemoteAddr(), p)
	}
	return c.Conn.Write(p)
}

func (c *conn) checkTLS() {
	preview, err := c.br.Peek(3)
	if err != nil || !looksLikeTLS(preview) {
		return
	}
	c.tls = true
	buf := make([]byte, 512)
	n, _ := c.br.Read(buf)
	c.buf = buf[:n]
	c.off = 0
	c.needHdr = false
	dumpTLS(c.component, c.RemoteAddr(), c.buf)
	logging.For(c.component).Warnf(
		"TLS handshake from %s (client is speaking HTTPS; NoSSL likely not running)",
		c.RemoteAddr(),
	)
}

func (c *conn) loadHeaders() error {
	var raw bytes.Buffer
	for {
		line, err := c.br.ReadBytes('\n')
		if len(line) > 0 {
			raw.Write(line)
		}
		if err != nil {
			if raw.Len() == 0 {
				// Keep needHdr so the next Read retries header mode.
				return err
			}
			c.buf = raw.Bytes()
			c.off = 0
			c.bodyLeft = 0
			c.needHdr = true
			dumpDir(c.component, "recv", c.RemoteAddr(), c.buf)
			return nil
		}
		if bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n")) {
			break
		}
		if raw.Len() > 64<<10 {
			c.buf = raw.Bytes()
			c.off = 0
			c.bodyLeft = 0
			c.needHdr = true
			dumpDir(c.component, "recv", c.RemoteAddr(), c.buf)
			return nil
		}
	}

	before := raw.Bytes()
	dumpDir(c.component, "recv", c.RemoteAddr(), before)
	c.buf = dedupeHost(before)
	c.off = 0
	c.bodyLeft = contentLength(c.buf)
	c.needHdr = c.bodyLeft == 0
	return nil
}

func contentLength(headerBlock []byte) int64 {
	for _, line := range bytes.Split(headerBlock, []byte("\n")) {
		trimmed := bytes.TrimSpace(bytes.TrimRight(line, "\r"))
		if len(trimmed) < 15 {
			continue
		}
		lower := strings.ToLower(string(trimmed))
		if !strings.HasPrefix(lower, "content-length:") {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(trimmed[len("content-length:"):])), 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		return n
	}
	return 0
}

func dedupeHost(headerBlock []byte) []byte {
	lines := bytes.SplitAfter(headerBlock, []byte("\n"))
	out := make([]byte, 0, len(headerBlock))
	seenHost := false
	for _, line := range lines {
		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) == 0 {
			out = append(out, line...)
			continue
		}
		if hasHostPrefix(trimmed) {
			if seenHost {
				continue
			}
			seenHost = true
		}
		out = append(out, line...)
	}
	return out
}

func hasHostPrefix(line []byte) bool {
	if len(line) < 5 {
		return false
	}
	return strings.EqualFold(string(line[:5]), "host:")
}
