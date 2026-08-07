// Package texttcp serves GameSpy text-protocol TCP connections.
package texttcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// Serve listens on TCP addr until ctx is cancelled, calling handle for each connection.
func Serve(ctx context.Context, addr, logComponent string, handle func(net.Conn)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	logging.For(logComponent).Infof("listening on %s", addr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logging.For(logComponent).Warnf("accept: %v", err)
			continue
		}
		logging.For(logComponent).Debugf("connection from=%s", conn.RemoteAddr())
		go handle(conn)
	}
}

// ReadLoop reads from conn until EOF or error, calling onChunk for each read.
// Normal disconnects (EOF, closed conn after logout) are silent, matching
// dwc_network_server_emulator connectionLost behavior.
func ReadLoop(conn net.Conn, logComponent string, onChunk func([]byte)) {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if !isNormalDisconnect(err) {
				logging.For(logComponent).Warnf("read %s: %v", conn.RemoteAddr(), err)
			}
			return
		}
		onChunk(buf[:n])
	}
}

func isNormalDisconnect(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}

// FrameBuffer accumulates incomplete GameSpy text frames across TCP reads.
type FrameBuffer struct {
	Remaining []byte
}

// Consume appends chunk, resyncs to \final\ if needed, and returns complete commands.
func (b *FrameBuffer) Consume(chunk []byte) []map[string]string {
	data := append(b.Remaining, chunk...)
	if len(data) > 0 && data[0] != '\\' {
		const final = "\\final\\"
		if idx := bytes.Index(data, []byte(final)); idx >= 0 {
			data = data[idx+len(final):]
		} else {
			b.Remaining = nil
			return nil
		}
	}

	commands, remainder := gamespy.ParseGameSpyMessage(data)
	b.Remaining = remainder
	return commands
}
