// Package profile implements the GameSpy profile (GPCM) TCP server.
package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"net"
	"strconv"
	"strings"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/texttcp"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// Server serves GameSpy profile login commands over TCP.
type Server struct {
	DB   database.Store
	Addr string
}

// Serve listens until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	return texttcp.Serve(ctx, s.Addr, "profile", s.handleConn)
}

type connSession struct {
	server    *Server
	conn      net.Conn
	challenge string
	sesskey   string
	profileid int64
	frames    texttcp.FrameBuffer
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	sess := &connSession{
		server: s,
		conn:   conn,
	}
	if err := sess.sendLoginChallenge(); err != nil {
		logging.For("profile").Errorf("login challenge: %v", err)
		return
	}

	texttcp.ReadLoop(conn, "profile", func(chunk []byte) {
		sess.dispatch(chunk)
	})
	sess.cleanup()
}

func (s *connSession) sendLoginChallenge() error {
	s.challenge = randomUpperAlpha(10)
	msg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "lc",
		"__cmd_val__": "1",
		"challenge":   s.challenge,
		"id":          "1",
	})
	_, err := s.conn.Write(msg)
	return err
}

func (s *connSession) dispatch(chunk []byte) {
	for _, cmd := range s.frames.Consume(chunk) {
		logging.For("profile").Debugf("inbound cmd=%s id=%s from=%s",
			cmd["__cmd__"], cmd["id"], s.conn.RemoteAddr())
		switch cmd["__cmd__"] {
		case "login":
			s.performLogin(cmd)
		case "getprofile":
			s.performGetProfile(cmd)
		case "updatepro":
			s.performUpdatePro(cmd)
		case "status":
			s.performStatus(cmd)
		case "ka":
			s.performKA()
		case "logout":
			s.performLogout(cmd)
			return
		default:
			logging.For("profile").Warnf("unknown command %q from %s", cmd["__cmd__"], s.conn.RemoteAddr())
		}
	}
}

func (s *connSession) performLogin(cmd map[string]string) {
	authtoken := cmd["authtoken"]
	authData, err := s.server.DB.GetNasLogin(authtoken)
	if err != nil || authData == nil {
		logging.For("profile").Warnf("login auth failure from %s: unknown or replaced authtoken", s.conn.RemoteAddr())
		s.writeError("266", "There was an error validating the pre-authentication.", cmd["id"])
		return
	}

	acChallenge := authData["challenge"]
	clientChallenge := cmd["challenge"]
	expected := gamespy.GenerateResponse(s.challenge, acChallenge, clientChallenge, authtoken)
	if cmd["response"] != expected {
		logging.For("profile").Warnf("invalid login response from %s: got %s want %s",
			s.conn.RemoteAddr(), cmd["response"], expected)
		s.writeError("256", "Login failed.", cmd["id"])
		return
	}

	proof := gamespy.GenerateProof(s.challenge, acChallenge, clientChallenge, authtoken)

	userid, profileid, _, uniquenick, err := s.server.DB.LoginProfileFromAuth(authData)
	if err != nil || profileid == 0 {
		logging.For("profile").Warnf("login profile from auth failed from %s: %v", s.conn.RemoteAddr(), err)
		s.writeError("256", "Login failed.", cmd["id"])
		return
	}

	sesskey, loginTicket, err := s.server.DB.CreateSession(profileid)
	if err != nil {
		logging.For("profile").Errorf("create session profileid=%d: %v", profileid, err)
		s.writeError("256", "Login failed.", cmd["id"])
		return
	}

	s.sesskey = sesskey
	s.profileid = profileid

	logging.For("profile").Infof("login success profileid=%d userid=%s", profileid, userid)

	msg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "lc",
		"__cmd_val__": "2",
		"sesskey":     sesskey,
		"proof":       proof,
		"userid":      userid,
		"profileid":   strconv.FormatInt(profileid, 10),
		"uniquenick":  uniquenick,
		"lt":          loginTicket,
		"id":          cmd["id"],
	})
	_, _ = s.conn.Write(msg)
}

func (s *connSession) performGetProfile(cmd map[string]string) {
	profileID, err := strconv.ParseInt(cmd["profileid"], 10, 64)
	if err != nil || profileID == 0 {
		logging.For("profile").Warnf("getprofile bad profileid %q from %s", cmd["profileid"], s.conn.RemoteAddr())
		s.writeError("256", "The profile was invalid.", cmd["id"])
		return
	}

	profile, err := s.server.DB.GetProfile(profileID)
	if err != nil || profile == nil {
		logging.For("profile").Warnf("getprofile missing profileid=%d from %s: %v", profileID, s.conn.RemoteAddr(), err)
		s.writeError("256", "The profile was invalid.", cmd["id"])
		return
	}

	sigBytes := make([]byte, 16)
	if _, err := rand.Read(sigBytes); err != nil {
		s.writeError("256", "The profile was invalid.", cmd["id"])
		return
	}

	fields := map[string]string{
		"__cmd__":     "pi",
		"__cmd_val__": "",
		"profileid":   strconv.FormatInt(profile.ProfileID, 10),
		"nick":        profile.Uniquenick,
		"userid":      profile.UserID,
		"email":       profile.Email,
		"sig":         hex.EncodeToString(sigBytes),
		"uniquenick":  profile.Uniquenick,
		"pid":         profile.PID,
		"lon":         profile.Lon,
		"lat":         profile.Lat,
		"loc":         profile.Loc,
		"id":          cmd["id"],
	}
	if profile.Firstname != "" {
		fields["firstname"] = profile.Firstname
	}
	if profile.Lastname != "" {
		fields["lastname"] = profile.Lastname
	}

	logging.For("profile").Infof("getprofile profileid=%d", profileID)
	_, _ = s.conn.Write(gamespy.CreateGameSpyMessage(fields))
}

func (s *connSession) performUpdatePro(cmd map[string]string) {
	if s.profileid == 0 {
		logging.For("profile").Warnf("updatepro before login from %s", s.conn.RemoteAddr())
		return
	}

	// Match dwc_network_server_emulator: only firstname/lastname are writable.
	fields := make(map[string]string)
	for key, value := range cmd {
		switch key {
		case "firstname", "lastname":
			fields[key] = value
		}
	}
	if len(fields) == 0 {
		return
	}

	if err := s.server.DB.UpdateProfile(s.profileid, fields); err != nil {
		logging.For("profile").Warnf("updatepro profileid=%d: %v", s.profileid, err)
		return
	}
	logging.For("profile").Infof("updatepro profileid=%d fields=%v", s.profileid, fields)
}

func (s *connSession) performStatus(cmd map[string]string) {
	if sesskey := cmd["sesskey"]; sesskey != "" {
		s.sesskey = sesskey
	}
	logging.For("profile").Infof("status profileid=%d status=%s", s.profileid, cmd["__cmd_val__"])
}

func (s *connSession) performKA() {
	logging.For("profile").Debugf("keepalive profileid=%d", s.profileid)
	msg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "ka",
		"__cmd_val__": "",
	})
	_, _ = s.conn.Write(msg)
}

func (s *connSession) performLogout(cmd map[string]string) {
	sesskey := cmd["sesskey"]
	if sesskey == "" {
		sesskey = s.sesskey
	}
	if sesskey != "" {
		logging.For("profile").Infof("logout profileid=%d", s.profileid)
		_ = s.server.DB.DeleteSession(sesskey)
		s.sesskey = ""
	}
	s.profileid = 0
	_ = s.conn.Close()
}

func (s *connSession) cleanup() {
	if s.sesskey != "" {
		logging.For("profile").Debugf("disconnect cleanup profileid=%d", s.profileid)
		_ = s.server.DB.DeleteSession(s.sesskey)
		s.sesskey = ""
	}
}

func (s *connSession) writeError(code, message, id string) {
	msg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "error",
		"__cmd_val__": "",
		"err":         code,
		"fatal":       "",
		"errmsg":      message,
		"id":          id,
	})
	_, _ = s.conn.Write(msg)
}

func randomUpperAlpha(n int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var b strings.Builder
	b.Grow(n)
	max := big.NewInt(int64(len(alphabet)))
	for range n {
		i, err := rand.Int(rand.Reader, max)
		if err != nil {
			b.WriteByte('A')
			continue
		}
		b.WriteByte(alphabet[i.Int64()])
	}
	return b.String()
}
