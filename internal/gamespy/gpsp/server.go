// Package gpsp implements the GameSpy player search (GPSP) TCP server.
package gpsp

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/texttcp"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// Server serves GameSpy Player Search (GPSP) over TCP (default :29901).
type Server struct {
	DB   database.Store
	Addr string
}

// Serve listens until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	return texttcp.Serve(ctx, s.Addr, "gpsp", s.handleConn)
}

type connSession struct {
	server *Server
	conn   net.Conn
	frames texttcp.FrameBuffer
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	sess := &connSession{
		server: s,
		conn:   conn,
	}

	texttcp.ReadLoop(conn, "gpsp", func(chunk []byte) {
		sess.dispatch(chunk)
	})
}

func (s *connSession) dispatch(chunk []byte) {
	for _, cmd := range s.frames.Consume(chunk) {
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
	_, hasNum := cmd["numopids"]
	msg := buildOtherslistReply(opids, hasNum, s.server.DB)

	count := 0
	if hasNum && opids != "" {
		count = len(strings.Split(opids, "|"))
	}
	logging.For("gpsp").Infof("otherslist from %s opids=%d", s.conn.RemoteAddr(), count)
	logging.For("gpsp").Debugf("otherslist reply from=%s opids=%d len=%d", s.conn.RemoteAddr(), count, len(msg))

	_, _ = s.conn.Write(msg)
}

func buildOtherslistReply(opids string, hasNumopids bool, db database.Store) []byte {
	pairs := []gamespy.KV{
		{Key: "otherslist", Value: ""},
	}

	if hasNumopids && opids != "" {
		for _, pidStr := range strings.Split(opids, "|") {
			pairs = append(pairs, gamespy.KV{Key: "o", Value: pidStr})

			// Match dwc_network_server_emulator: missing profiles (including
			// sentinel opid 0) get an empty uniquenick with no error log.
			uniquenick := ""
			pid, err := strconv.ParseInt(pidStr, 10, 64)
			if err != nil {
				logging.For("gpsp").Warnf("otherslist bad opid %q: %v", pidStr, err)
			} else if pid != 0 {
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
