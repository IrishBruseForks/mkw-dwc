// Package gpsp implements the GameSpy player search (GPSP) TCP server.
package gpsp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// Server serves GameSpy Player Search (GPSP) over TCP (default :29901).
type Server struct {
	DB   database.Store
	Addr string
}

// Serve listens until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	logging.For("gpsp").Infof("listening on %s", s.Addr)

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
			var ne net.Error
			if errors.As(err, &ne) && ne.Temporary() {
				continue
			}
			return err
		}
		go s.handleConn(conn)
	}
}

type connSession struct {
	server    *Server
	conn      net.Conn
	remaining []byte
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	sess := &connSession{
		server: s,
		conn:   conn,
	}

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logging.For("gpsp").Errorf("read %s: %v", conn.RemoteAddr(), err)
			}
			return
		}
		sess.dispatch(buf[:n])
	}
}

func (s *connSession) dispatch(chunk []byte) {
	data := append(s.remaining, chunk...)
	if len(data) > 0 && data[0] != '\\' {
		const final = "\\final\\"
		if idx := bytes.Index(data, []byte(final)); idx >= 0 {
			data = data[idx+len(final):]
		} else {
			s.remaining = nil
			return
		}
	}

	commands, remainder := gamespy.ParseGameSpyMessage(data)
	s.remaining = remainder

	for _, cmd := range commands {
		switch cmd["__cmd__"] {
		case "otherslist":
			s.performOtherslist(cmd)
		default:
			logging.For("gpsp").Warnf("unknown command %q from %s", cmd["__cmd__"], s.conn.RemoteAddr())
		}
	}
}

func (s *connSession) performOtherslist(cmd map[string]string) {
	opids := cmd["opids"]
	msg := buildOtherslistReply(opids, s.server.DB)

	count := 0
	if opids != "" {
		count = len(strings.Split(opids, "|"))
	}
	logging.For("gpsp").Infof("otherslist from %s opids=%d", s.conn.RemoteAddr(), count)

	_, _ = s.conn.Write(msg)
}

func buildOtherslistReply(opids string, db database.Store) []byte {
	pairs := []gamespy.KV{
		{Key: "otherslist", Value: ""},
	}

	if opids != "" {
		for _, pidStr := range strings.Split(opids, "|") {
			pairs = append(pairs, gamespy.KV{Key: "o", Value: pidStr})

			uniquenick := ""
			if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
				if profile, err := db.GetProfile(pid); err == nil && profile != nil {
					uniquenick = profile.Uniquenick
				}
			}
			pairs = append(pairs, gamespy.KV{Key: "uniquenick", Value: uniquenick})
		}
	}

	pairs = append(pairs, gamespy.KV{Key: "oldone", Value: ""})
	return gamespy.CreateGameSpyMessageOrdered(pairs)
}
