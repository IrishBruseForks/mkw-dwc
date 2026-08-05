
// Package qr implements the GameSpy Query & Reporting UDP server.
package qr

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// IngameSNLookup resolves ingamesn values for QR heartbeats.
type IngameSNLookup interface {
	GetIngameSN(profileID int64) (string, error)
}

// Server serves GameSpy QR (Query & Reporting) over UDP.
type Server struct {
	Addr                  string
	Backend               *backend.Backend
	Keys                  map[string]string
	Profiles              IngameSNLookup
	RewriteDolphinLocalIP bool // rewrite Dolphin localip0=10.0.1.30 to UDP source IP

	mu       sync.Mutex
	sessions map[uint32]*qrSession
	writeCh  chan writeItem
}

type qrSession struct {
	addr          net.UDPAddr
	sessionID     uint32
	challenge     string
	secretkey     string
	sentChallenge bool
	heartbeatData map[string]string
	console       int
	playerid      int64
	gamename      string
	ingamesn      string
	keepalive     time.Time
}

type writeItem struct {
	data []byte
	addr net.UDPAddr
}

// New returns a QR server bound to addr with the given backend and secret keys.
func New(addr string, backend *backend.Backend, keys map[string]string) *Server {
	return &Server{
		Addr:     addr,
		Backend:  backend,
		Keys:     keys,
		sessions: make(map[uint32]*qrSession),
	}
}

// Serve listens on UDP until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	conn, err := net.ListenPacket("udp", s.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if s.sessions == nil {
		s.sessions = make(map[uint32]*qrSession)
	}

	s.writeCh = make(chan writeItem, 64)
	go s.writeWorker(ctx, conn)

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.keepaliveCheck()
			}
		}
	}()

	logging.For("qr").Infof("listening on %s", s.Addr)

	buf := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}

		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}
		s.handlePacket(buf[:n], *udpAddr)
	}
}

// ForwardClientMessage relays a server-browser message to a QR client as FE FD 06.
func (s *Server) ForwardClientMessage(session, cookie uint32, dest net.UDPAddr, payload []byte) error {
	packet := make([]byte, 3+4+4+len(payload))
	packet[0] = 0xfe
	packet[1] = 0xfd
	packet[2] = 0x06
	binary.LittleEndian.PutUint32(packet[3:], session)
	binary.LittleEndian.PutUint32(packet[7:], cookie)
	copy(packet[11:], payload)

	s.queueWrite(packet, dest)
	return nil
}

func (s *Server) writeWorker(ctx context.Context, conn net.PacketConn) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-s.writeCh:
			_, err := conn.WriteTo(item.data, &item.addr)
			if err != nil && ctx.Err() == nil {
				logging.For("qr").Errorf("write to %s: %v", item.addr.String(), err)
			}
		}
	}
}

func (s *Server) queueWrite(data []byte, addr net.UDPAddr) {
	if s.writeCh == nil {
		return
	}
	select {
	case s.writeCh <- writeItem{data: data, addr: addr}:
	default:
		logging.For("qr").Warnf("write queue full, dropping packet to %s", addr.String())
	}
}

func (s *Server) handlePacket(recvData []byte, addr net.UDPAddr) {
	if len(recvData) == 0 {
		return
	}

	cmd := recvData[0]
	var sessionID uint32
	var sessionIDRaw []byte

	if cmd != 0x09 {
		if len(recvData) < 5 {
			return
		}
		sessionID = binary.LittleEndian.Uint32(recvData[1:5])
		sessionIDRaw = recvData[1:5]

		s.mu.Lock()
		sess, ok := s.sessions[sessionID]
		if !ok {
			sess = &qrSession{
				addr:      addr,
				sessionID: sessionID,
				keepalive: time.Now(),
			}
			s.sessions[sessionID] = sess
		}
		sess.addr = addr
		sess.keepalive = time.Now()
		s.mu.Unlock()
	}

	switch cmd {
	case 0x01:
		s.handleChallengeResponse(sessionID, recvData, addr)
	case 0x03:
		s.handleHeartbeat(sessionID, sessionIDRaw, recvData, addr)
	case 0x07:
		// Client message ack for FE FD 06 (Python logs at DEBUG only).
	case 0x08:
		// keepalive refresh already applied above
	case 0x09:
		s.handleAvailability(recvData, addr)
	default:
		logging.For("qr").Warnf("unknown command 0x%02x from %s", cmd, addr.String())
	}
}

func (s *Server) handleAvailability(recvData []byte, addr net.UDPAddr) {
	s.queueWrite([]byte{0xfe, 0xfd, 0x09, 0x00, 0x00, 0x00, 0x00}, addr)
}

func (s *Server) handleChallengeResponse(sessionID uint32, recvData []byte, addr net.UDPAddr) {
	if len(recvData) < 6 {
		return
	}

	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	secretkey := sess.secretkey
	challenge := sess.challenge
	heartbeatData := sess.heartbeatData
	s.mu.Unlock()

	if len(recvData) < 6 {
		logging.For("qr").Warnf("challenge too short session=%08x from %s len=%d", sessionID, addr.String(), len(recvData))
		return
	}
	clientChallenge := string(recvData[5 : len(recvData)-1])

	expected := gamespy.PrepareRC4Base64(secretkey, challenge)
	if clientChallenge != expected {
		logging.For("qr").Warnf("challenge mismatch session=%08x from %s", sessionID, addr.String())
		s.mu.Lock()
		if sess, ok := s.sessions[sessionID]; ok {
			sess.sentChallenge = false
			if sess.gamename != "" {
				s.Backend.DeleteServer(sess.gamename, sessionID)
			}
		}
		s.mu.Unlock()
		return
	}

	packet := make([]byte, 7)
	packet[0] = 0xfe
	packet[1] = 0xfd
	packet[2] = 0x0a
	binary.LittleEndian.PutUint32(packet[3:], sessionID)
	s.queueWrite(packet, addr)

	if heartbeatData != nil {
		s.updateServerList(sessionID, heartbeatData)
	}
}

func (s *Server) handleHeartbeat(sessionID uint32, sessionIDRaw []byte, recvData []byte, addr net.UDPAddr) {
	if len(recvData) < 6 {
		return
	}

	k := parseNullDelimitedKV(recvData[5:])

	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}

	if gamename, ok := k["gamename"]; ok {
		if key, found := s.Keys[gamename]; found {
			sess.secretkey = key
		} else {
			logging.For("qr").Warnf("connection from unknown game %q (session %08x)", gamename, sessionID)
		}
		sess.console = detectConsole(gamename)
	}

	if sess.playerid == 0 {
		if pidStr, ok := k["dwc_pid"]; ok {
			if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
				sess.playerid = pid
			}
		}
	}

	if publicip, ok := k["publicip"]; ok && publicip == "0" {
		k["publicip"] = gamespy.SignedIPString(addr.IP, sess.console)
	}

	// Dolphin always reports localip0=10.0.1.30. That address is not a real
	// local interface. When the server and clients share a public IP (typical
	// loopback /etc/hosts setup), MKWii skips NATNEG and tries LAN connect via
	// localip0, which never works for two Dolphin instances. Rewrite to the
	// real UDP source IP so same-machine clients can reach each other.
	if s.RewriteDolphinLocalIP {
		if localip0, ok := k["localip0"]; ok && localip0 == "10.0.1.30" {
			if ip4 := addr.IP.To4(); ip4 != nil {
				fixed := ip4.String()
				logging.For("qr").Debugf("dolphin localip0 rewrite session=%08x %s -> %s", sessionID, localip0, fixed)
				k["localip0"] = fixed
			}
		}
	}

	if publicport, ok := k["publicport"]; ok {
		if localport, lok := k["localport"]; lok && publicport != localport {
			k["publicport"] = strconv.Itoa(addr.Port)
		}
	}

	if s.Profiles != nil {
		if _, ok := k["gamename"]; ok {
			if pidStr, ok := k["dwc_pid"]; ok {
				if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
					if sn, err := s.Profiles.GetIngameSN(pid); err == nil && sn != "" {
						sess.ingamesn = sn
					}
				}
			}
		}
		if sess.ingamesn != "" {
			if _, ok := k["ingamesn"]; !ok {
				k["ingamesn"] = sess.ingamesn
			}
		}
	}
	sentChallenge := sess.sentChallenge
	s.mu.Unlock()

	if sentChallenge {
		s.updateServerList(sessionID, k)
		return
	}

	addrHex := formatIPHex(addr.IP)
	portHex := fmt.Sprintf("%04X", addr.Port)
	serverChallenge := randomAlphaNum(6) + "00" + addrHex + portHex

	s.mu.Lock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.challenge = serverChallenge
		sess.sentChallenge = true
		sess.heartbeatData = maps.Clone(k)
	}
	s.mu.Unlock()

	packet := make([]byte, 3+4+len(serverChallenge)+1)
	packet[0] = 0xfe
	packet[1] = 0xfd
	packet[2] = 0x01
	copy(packet[3:], sessionIDRaw)
	copy(packet[7:], serverChallenge)
	packet[len(packet)-1] = 0
	s.queueWrite(packet, addr)
}

func (s *Server) updateServerList(sessionID uint32, k map[string]string) {
	if state, ok := k["statechanged"]; ok && state == "2" {
		gamename := k["gamename"]
		if gamename == "" {
			s.mu.Lock()
			if sess, ok := s.sessions[sessionID]; ok {
				gamename = sess.gamename
			}
			s.mu.Unlock()
		}
		logging.For("qr").Infof("room removed gamename=%s session=%08x", gamename, sessionID)
		s.Backend.DeleteServer(gamename, sessionID)

		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return
	}

	gamename, ok := k["gamename"]
	if !ok {
		return
	}

	s.mu.Lock()
	sess, found := s.sessions[sessionID]
	console := 0
	if found {
		console = sess.console
	}
	s.mu.Unlock()

	_ = s.Backend.UpdateServerList(gamename, sessionID, k, console)
	logging.For("qr").Debugf(
		"room registered gamename=%s session=%08x dwc_pid=%s hoststate=%s mtype=%s suspend=%s rk=%s ev=%s publicip=%s publicport=%s numplayers=%s",
		gamename,
		sessionID,
		k["dwc_pid"],
		k["dwc_hoststate"],
		k["dwc_mtype"],
		k["dwc_suspend"],
		k["rk"],
		k["ev"],
		k["publicip"],
		k["publicport"],
		k["numplayers"],
	)

	s.mu.Lock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.gamename = gamename
	}
	s.mu.Unlock()
}

func (s *Server) keepaliveCheck() {
	now := time.Now()
	timeout := 61 * time.Second

	s.mu.Lock()
	var pruned []uint32
	for id, sess := range s.sessions {
		delta := now.Sub(sess.keepalive)
		if delta < 0 || delta >= timeout {
			logging.For("qr").Debugf("pruning session gamename=%s session=%08x", sess.gamename, id)
			pruned = append(pruned, id)
			if sess.gamename != "" {
				s.Backend.DeleteServer(sess.gamename, id)
			}
		}
	}
	for _, id := range pruned {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
}

func parseNullDelimitedKV(data []byte) map[string]string {
	s := string(bytesTrimRight(data, 0))
	parts := strings.Split(s, "\x00")
	k := make(map[string]string)
	for i := 0; i+1 < len(parts); i += 2 {
		k[parts[i]] = parts[i+1]
	}
	return k
}

func bytesTrimRight(data []byte, b byte) []byte {
	for len(data) > 0 && data[len(data)-1] == b {
		data = data[:len(data)-1]
	}
	return data
}

func detectConsole(gamename string) int {
	if strings.HasSuffix(gamename, "ds") ||
		strings.HasSuffix(gamename, "dsam") ||
		strings.HasSuffix(gamename, "dsi") ||
		strings.HasSuffix(gamename, "dsiam") {
		return 0
	}
	if strings.HasSuffix(gamename, "wii") ||
		strings.HasSuffix(gamename, "wiiam") ||
		strings.HasSuffix(gamename, "wiiware") ||
		strings.HasSuffix(gamename, "wiiwaream") {
		return 1
	}
	return 0
}

func formatIPHex(ip net.IP) string {
	v4 := ip.To4()
	if v4 == nil {
		return "00000000"
	}
	return fmt.Sprintf("%02X%02X%02X%02X", v4[0], v4[1], v4[2], v4[3])
}

func randomAlphaNum(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	b.Grow(n)
	max := big.NewInt(int64(len(alphabet)))
	for range n {
		i, err := rand.Int(rand.Reader, max)
		if err != nil {
			b.WriteByte('a')
			continue
		}
		b.WriteByte(alphabet[i.Int64()])
	}
	return b.String()
}
