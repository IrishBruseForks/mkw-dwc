// Package natneg implements the GameSpy NAT Negotiation UDP server (v0x03).
package natneg

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

const (
	nnVersion       = 0x03
	sessionTTL      = 30 * time.Minute
	sendDelay       = 50 * time.Millisecond
	cleanupInterval = time.Minute
)

var nnMagic = []byte{0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2}

var initAckTail = []byte{0xff, 0xff, 0x6d, 0x16, 0xb5, 0x7d, 0xea}

const (
	recInit         = 0x00
	recInitAck      = 0x01
	recERTTest      = 0x02
	recConnect      = 0x05
	recConnectAck   = 0x06
	recBackupTest   = 0x08
	recBackupAck    = 0x09
	recAddressCheck = 0x0a
	recAddressReply = 0x0b
	recNatify       = 0x0c
	recReport       = 0x0d
	recReportAck    = 0x0e
)

// Server serves GameSpy NAT Negotiation for MKWii (protocol v0x03).
type Server struct {
	Addr    string
	Backend *backend.Backend

	mu       sync.Mutex
	sessions map[uint32]*sessionState

	writeCh chan outboundMsg
}

type sessionState struct {
	clients      map[byte]*clientSlot
	lastActivity time.Time
}

type clientSlot struct {
	connected  bool
	addr       *net.UDPAddr
	localAddr  backend.LocalAddr
	serverAddr map[string]string
	gameID     string
}

type outboundMsg struct {
	data []byte
	addr *net.UDPAddr
}

type clientInfo struct {
	index     byte
	addr      *net.UDPAddr
	localAddr backend.LocalAddr
	gameID    string
}

type connectPair struct {
	a, b   clientInfo
	header [12]byte
}

// New returns a NATNEG server bound to addr using backend for server lookups.
func New(addr string, backend *backend.Backend) *Server {
	return &Server{
		Addr:     addr,
		Backend:  backend,
		sessions: make(map[uint32]*sessionState),
	}
}

// Serve listens on UDP until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	conn, err := net.ListenPacket("udp", s.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	logging.For("natneg").Infof("listening on %s", s.Addr)

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		return errors.New("natneg: expected UDP connection")
	}

	s.writeCh = make(chan outboundMsg, 256)
	go s.writeQueueWorker(ctx, udpConn)
	go s.sessionCleanup(ctx)

	go func() {
		<-ctx.Done()
		_ = udpConn.Close()
	}()

	buf := make([]byte, 2048)
	for {
		n, addr, err := udpConn.ReadFromUDP(buf)
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

		s.handlePacket(buf[:n], addr)
	}
}

func (s *Server) writeQueueWorker(ctx context.Context, conn *net.UDPConn) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.writeCh:
			timer := time.NewTimer(sendDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, err := conn.WriteToUDP(msg.data, msg.addr); err != nil && ctx.Err() == nil {
				logging.For("natneg").Errorf("write %s: %v", msg.addr, err)
			}
		}
	}
}

func (s *Server) enqueue(data []byte, addr *net.UDPAddr) {
	if s.writeCh == nil {
		return
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case s.writeCh <- outboundMsg{data: cp, addr: addr}:
	default:
		logging.For("natneg").Warnf("write queue full, dropping packet to %s", addr)
	}
}

func (s *Server) sessionCleanup(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			var expired []uint32
			for id, sess := range s.sessions {
				if now.Sub(sess.lastActivity) > sessionTTL {
					expired = append(expired, id)
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
			if len(expired) > 0 {
				logging.For("natneg").Debugf("expired %d session(s): %v", len(expired), expired)
			}
			for _, id := range expired {
				s.Backend.DeleteNatnegServer(id)
			}
		}
	}
}

func (s *Server) handlePacket(data []byte, addr *net.UDPAddr) {
	if len(data) < 8 || data[6] != nnVersion {
		return
	}
	if !hasMagic(data) {
		logging.For("natneg").Warnf("illegal packet from %s", addr)
		return
	}

	recType := data[7]
	switch recType {
	case recInit:
		s.handleInit(data, addr)
	case recConnectAck:
		s.handleConnectAck(data, addr)
	case recBackupTest:
		s.handleBackupTest(data, addr)
	case recAddressCheck:
		s.handleAddressCheck(data, addr)
	case recNatify:
		s.handleNatify(data, addr)
	case recReport:
		s.handleReport(data, addr)
	default:
		logging.For("natneg").Warnf("unknown command %02x from %s", recType, addr)
	}
}

func (s *Server) handleInit(data []byte, addr *net.UDPAddr) {
	if len(data) < 22 {
		return
	}

	sessionID := binary.LittleEndian.Uint32(data[8:12])
	clientIndex := data[13]
	gameID := readCString(data, 0x15)
	localAddr := parseLocalAddr(data, 15)

	logging.For("natneg").Infof("INIT session=%08x client=%d game=%q", sessionID, clientIndex, gameID)

	initAck := make([]byte, 14+len(initAckTail))
	copy(initAck, data[:14])
	copy(initAck[14:], initAckTail)
	initAck[7] = recInitAck
	s.enqueue(initAck, addr)

	var pairs []connectPair

	s.mu.Lock()
	sess := s.sessions[sessionID]
	if sess == nil {
		sess = &sessionState{
			clients:      make(map[byte]*clientSlot),
			lastActivity: time.Now(),
		}
		s.sessions[sessionID] = sess
	}
	sess.lastActivity = time.Now()

	slot := sess.clients[clientIndex]
	if slot == nil {
		slot = &clientSlot{}
		sess.clients[clientIndex] = slot
	}
	slot.addr = cloneUDPAddr(addr)
	slot.localAddr = localAddr
	slot.gameID = gameID
	slot.connected = false

	for otherIndex, other := range sess.clients {
		if otherIndex == clientIndex || other.connected {
			continue
		}
		if other.addr == nil || slot.addr == nil {
			continue
		}

		var header [12]byte
		copy(header[:], data[:12])

		pairs = append(pairs, connectPair{
			a: clientInfo{
				index:     clientIndex,
				addr:      cloneUDPAddr(slot.addr),
				localAddr: localAddr,
				gameID:    gameID,
			},
			b: clientInfo{
				index:     otherIndex,
				addr:      cloneUDPAddr(other.addr),
				localAddr: other.localAddr,
				gameID:    other.gameID,
			},
			header: header,
		})
	}
	s.mu.Unlock()

	serverAddrs := make(map[byte]map[string]string)
	for _, pair := range pairs {
		for _, ci := range []clientInfo{pair.a, pair.b} {
			if _, ok := serverAddrs[ci.index]; ok {
				continue
			}
			serverAddrs[ci.index] = s.lookupServerAddr(ci.gameID, sessionID, ci.addr, ci.localAddr)
		}
	}

	if len(serverAddrs) > 0 {
		s.mu.Lock()
		if sess := s.sessions[sessionID]; sess != nil {
			for idx, sa := range serverAddrs {
				if c := sess.clients[idx]; c != nil {
					c.serverAddr = sa
				}
			}
		}
		s.mu.Unlock()
	}

	for _, pair := range pairs {
		s.enqueue(
			buildConnectPacket(pair.header[:], pair.b.addr, publicPort(serverAddrs[pair.b.index], pair.b.localAddr, pair.b.addr)),
			pair.a.addr,
		)
		s.enqueue(
			buildConnectPacket(pair.header[:], pair.a.addr, publicPort(serverAddrs[pair.a.index], pair.a.localAddr, pair.a.addr)),
			pair.b.addr,
		)
	}
}

func (s *Server) handleConnectAck(data []byte, addr *net.UDPAddr) {
	if len(data) < 14 {
		return
	}

	sessionID := binary.LittleEndian.Uint32(data[8:12])
	clientIndex := data[13]

	s.mu.Lock()
	defer s.mu.Unlock()

	sess := s.sessions[sessionID]
	if sess == nil {
		return
	}
	sess.lastActivity = time.Now()

	slot := sess.clients[clientIndex]
	if slot == nil {
		return
	}
	slot.connected = true
	logging.For("natneg").Infof("CONNECT session=%08x client=%d", sessionID, clientIndex)
}

func (s *Server) handleBackupTest(data []byte, addr *net.UDPAddr) {
	out := make([]byte, len(data))
	copy(out, data)
	out[7] = recBackupAck
	s.enqueue(out, addr)
}

func (s *Server) handleAddressCheck(data []byte, addr *net.UDPAddr) {
	if len(data) < 15 {
		return
	}

	out := make([]byte, len(data))
	copy(out, data[:15])

	ip4 := addr.IP.To4()
	if ip4 == nil {
		return
	}
	copy(out[15:19], ip4)
	binary.BigEndian.PutUint16(out[19:21], uint16(addr.Port))
	if len(data) > 21 {
		copy(out[21:], data[21:])
	}
	out[7] = recAddressReply
	s.enqueue(out, addr)
}

func (s *Server) handleNatify(data []byte, addr *net.UDPAddr) {
	out := make([]byte, len(data))
	copy(out, data)
	out[7] = recERTTest
	s.enqueue(out, addr)
}

func (s *Server) handleReport(data []byte, addr *net.UDPAddr) {
	if len(data) < 21 {
		return
	}

	out := make([]byte, 21)
	copy(out, data[:21])
	out[7] = recReportAck
	out[14] = 0
	s.enqueue(out, addr)
}

func (s *Server) lookupServerAddr(gameID string, sessionID uint32, addr *net.UDPAddr, localAddr backend.LocalAddr) map[string]string {
	if addr == nil {
		return nil
	}

	ipStr := addr.IP.String()
	servers := s.Backend.GetNatnegServer(sessionID)
	for _, server := range servers {
		if server["publicip"] == ipStr {
			return server
		}
		if gamespy.MatchPublicIP(server["publicip"], ipStr) {
			return server
		}
	}

	rec := s.Backend.FindServerByLocalAddress(ipStr, localAddr, gameID)
	if rec == nil {
		rec = s.Backend.FindServerByAddress(ipStr, addr.Port, gameID)
	}
	if rec == nil {
		return nil
	}
	return rec.AsMap()
}

func buildConnectPacket(template []byte, remote *net.UDPAddr, publicPort uint16) []byte {
	out := make([]byte, 20)
	copy(out, template[:12])
	ip4 := remote.IP.To4()
	if ip4 != nil {
		copy(out[12:16], ip4)
	}
	binary.BigEndian.PutUint16(out[16:18], publicPort)
	out[18] = 0x42
	out[19] = 0x00
	out[7] = recConnect
	return out
}

func publicPort(serverAddr map[string]string, localAddr backend.LocalAddr, addr *net.UDPAddr) uint16 {
	if serverAddr != nil {
		if p, err := strconv.Atoi(serverAddr["publicport"]); err == nil && p > 0 {
			return uint16(p)
		}
	}
	if localAddr.Port != 0 {
		return localAddr.Port
	}
	if addr != nil {
		return uint16(addr.Port)
	}
	return 0
}

func parseLocalAddr(data []byte, off int) backend.LocalAddr {
	var la backend.LocalAddr
	if len(data) < off+6 {
		return la
	}
	copy(la.IP[:], data[off:off+4])
	la.Port = binary.BigEndian.Uint16(data[off+4 : off+6])
	return la
}

func readCString(data []byte, off int) string {
	if off >= len(data) {
		return ""
	}
	end := off
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[off:end])
}

func hasMagic(data []byte) bool {
	if len(data) < len(nnMagic) {
		return false
	}
	for i, b := range nnMagic {
		if data[i] != b {
			return false
		}
	}
	return true
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	cp := *addr
	return &cp
}

