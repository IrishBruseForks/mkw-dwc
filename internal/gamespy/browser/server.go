// Package browser implements the GameSpy server browser TCP protocol.
package browser

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"unicode"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

const (
	maxPacketLength = 256 + 511 + 255 // 1022, OpenSpy chunk size
	maxBufferSize   = maxPacketLength * 4

	cmdServerList = 0x00
	cmdSendMsg    = 0x02
	cmdKeepAlive  = 0x03

	optNoServerList      = 0x02
	optAlternateSourceIP = 0x08
	optLimitResultCount  = 0x80

	unsolicitedUDPFlag         = 1
	privateIPFlag              = 2
	connectNegotiateFlag       = 4
	icmpIPFlag                 = 8
	nonstandardPortFlag        = 16
	nonstandardPrivatePortFlag = 32
	hasKeysFlag                = 64
)

var (
	sbcmMagic   = []byte{0x53, 0x42, 0x43, 0x4d}
	altMagic    = []byte{0xbb, 0x49, 0xcc, 0x4d}
	natnegMagic = []byte{0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2}
)

// Relay forwards server-browser messages to QR clients as FE FD 06 packets.
type Relay interface {
	ForwardClientMessage(session, cookie uint32, dest net.UDPAddr, payload []byte) error
}

// Server serves GameSpy server browser commands over TCP.
type Server struct {
	Addr    string
	Backend *backend.Backend
	Keys    map[string]string
	Relay   Relay
}

// New returns a browser server bound to addr.
func New(addr string, backend *backend.Backend, keys map[string]string, relay Relay) *Server {
	return &Server{
		Addr:    addr,
		Backend: backend,
		Keys:    keys,
		Relay:   relay,
	}
}

// Serve listens until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

		logging.For("browser").Infof("listening on %s (lists rooms registered via QR on :27900)", s.Addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logging.For("browser").Warnf("accept: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

type connSession struct {
	server    *Server
	conn      net.Conn
	console   int
	ownServer *backend.ServerRecord
	buffer    []byte
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	logging.For("browser").Debugf("connection from %s", conn.RemoteAddr())

	sess := &connSession{
		server: s,
		conn:   conn,
	}

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				logging.For("browser").Warnf("read %s: %v", conn.RemoteAddr(), err)
			}
			return
		}
		if !sess.dispatch(buf[:n]) {
			return
		}
	}
}

func (sess *connSession) dispatch(chunk []byte) bool {
	sess.buffer = append(sess.buffer, chunk...)
	if len(sess.buffer) > maxBufferSize {
		return false
	}

	for len(sess.buffer) > 0 {
		if len(sess.buffer) < 2 {
			return true
		}

		packetLen := int(gamespy.ReadU16BE(sess.buffer))
		if packetLen < 3 || packetLen > maxBufferSize {
			return false
		}
		if len(sess.buffer) < packetLen {
			return true
		}

		packet := sess.buffer[:packetLen]
		sess.buffer = sess.buffer[packetLen:]

		if len(packet) < 3 {
			continue
		}

		switch packet[2] {
		case cmdServerList:
			sess.handleServerList(packet)
		case cmdSendMsg:
			sess.handleSendMessage(packet)
		case cmdKeepAlive:
		default:
			logging.For("browser").Warnf("unknown command %02x from %s", packet[2], sess.conn.RemoteAddr())
		}
	}
	return true
}

func (sess *connSession) handleServerList(packet []byte) {
	if len(packet) < 3 {
		return
	}

	idx := 3
	if idx+6 > len(packet) {
		return
	}
	idx += 2 // list version, encoding version
	_ = binary.LittleEndian.Uint32(packet[idx : idx+4])
	idx += 4

	queryGame, nextIdx := gamespy.ReadCString(packet, idx)
	if nextIdx < 0 {
		return
	}
	idx = nextIdx

	gameName, nextIdx := gamespy.ReadCString(packet, idx)
	if nextIdx < 0 {
		return
	}
	idx = nextIdx

	if idx+8 > len(packet) {
		return
	}
	challenge := string(packet[idx : idx+8])
	idx += 8

	filter, nextIdx := gamespy.ReadCString(packet, idx)
	if nextIdx < 0 {
		return
	}
	idx = nextIdx

	fieldsStr, nextIdx := gamespy.ReadCString(packet, idx)
	if nextIdx < 0 {
		return
	}
	idx = nextIdx

	if idx+4 > len(packet) {
		return
	}
	// Options are big-endian (Python reference: get_int(..., True)).
	options := binary.BigEndian.Uint32(packet[idx : idx+4])
	idx += 4

	maxServers := 0
	sendIP := false
	// Match Python's if/elif chain: only one of these option payloads applies.
	switch {
	case options&optLimitResultCount != 0:
		if idx+4 <= len(packet) {
			maxServers = int(binary.LittleEndian.Uint32(packet[idx : idx+4]))
			idx += 4
		}
	case options&optAlternateSourceIP != 0:
		if idx+4 <= len(packet) {
			idx += 4
		}
	case options&optNoServerList != 0:
		sendIP = true
	}

	var fields []string
	if strings.Contains(fieldsStr, `\`) {
		for _, part := range strings.Split(fieldsStr, `\`) {
			if part != "" && !isOnlySpace(part) {
				fields = append(fields, part)
			}
		}
	} else if fieldsStr != "" {
		fields = []string{fieldsStr}
	}

	if (filter == "" && len(fields) == 0) || sendIP {
		logging.For("browser").Debugf("own-ip check from %s game=%q", sess.conn.RemoteAddr(), gameName)
		sess.sendOwnIP(gameName, challenge)
		return
	}

	logging.For("browser").Debugf("server list query from %s game=%q filter=%q fields=%v max=%d", sess.conn.RemoteAddr(), queryGame, filter, fields, maxServers)
	sess.findServer(queryGame, filter, fields, maxServers, gameName, challenge)
}

func (sess *connSession) sendOwnIP(gameName, challenge string) {
	host, _, err := net.SplitHostPort(sess.conn.RemoteAddr().String())
	if err != nil {
		return
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return
	}

	out := make([]byte, 0, 6)
	out = append(out, ip[0], ip[1], ip[2], ip[3])
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, 6500)
	out = append(out, portBuf...)

	secret := sess.server.Keys[gameName]
	if secret == "" {
		logging.For("browser").Warnf("missing secret key for game %q (own-ip)", gameName)
	}
	enc := gamespy.EncTypeXEncrypt(secret, challenge, out)
	_, _ = sess.conn.Write(enc)
}

func (sess *connSession) findServer(queryGame, filter string, fields []string, maxServers int, gameName, challenge string) {
	results, err := sess.server.Backend.FindServers(queryGame, filter, fields, maxServers)
	if err != nil {
		logging.For("browser").Warnf("FindServers game=%q filter=%q: %v (sending empty list)", queryGame, filter, err)
		results = nil
	}
	logging.For("browser").Debugf("returning %d room(s) for game=%q", len(results), queryGame)
	if len(results) == 0 && filter != "" {
		logging.For("browser").Debugf("zero rooms matched game=%q filter=%q", queryGame, filter)
	}
	for i, result := range results {
		fm := result.Record.AsMap()
		logging.For("browser").Debugf(
			"  room[%d] session=%08x dwc_pid=%s hoststate=%s mtype=%s suspend=%s rk=%s ev=%s publicip=%s requested=%v",
			i,
			result.Record.SessionID,
			fm["dwc_pid"],
			fm["dwc_hoststate"],
			fm["dwc_mtype"],
			fm["dwc_suspend"],
			fm["rk"],
			fm["ev"],
			result.Record.PublicIP,
			result.Requested,
		)
	}
	if len(results) == 0 {
		results = []backend.ServerResult{{}}
	}

	data := sess.generateServerListHeader(fields)

	for _, result := range results {
		server := result
		if shouldDropRequestedRoom(fields, server.Requested) {
			logging.For("browser").Warnf("dropping room session=%08x: requested fields missing", result.Record.SessionID)
			server = backend.ServerResult{}
		} else if server.Record.SessionID != 0 || server.Record.PublicIP != "" {
			sess.console = server.Record.Console
		}

		data = append(data, generateServerEntry(fields, server, server.Record.Console)...)

		if len(data) >= maxPacketLength {
			sess.sendEncrypted(gameName, challenge, data)
			data = nil
		}
	}

	term := make([]byte, 5)
	term[0] = 0
	binary.LittleEndian.PutUint32(term[1:], 0xffffffff)
	data = append(data, term...)

	sess.sendEncrypted(gameName, challenge, data)
}

func (sess *connSession) generateServerListHeader(fields []string) []byte {
	host, portStr, err := net.SplitHostPort(sess.conn.RemoteAddr().String())
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return nil
	}
	port, _ := strconv.Atoi(portStr)

	out := make([]byte, 0, 64+len(fields)*16)
	out = append(out, ip[0], ip[1], ip[2], ip[3])
	// Client TCP port is big-endian (Python: get_bytes_from_short(port, True)).
	portBuf := make([]byte, 2)
	gamespy.WriteU16BE(portBuf, uint16(port))
	out = append(out, portBuf...)

	// Field count is little-endian (Python: get_bytes_from_short(key_count)).
	keyCount := make([]byte, 2)
	gamespy.WriteU16LE(keyCount, uint16(len(fields)))
	out = append(out, keyCount...)

	for _, field := range fields {
		out = append(out, field...)
		out = append(out, 0, 0)
	}
	return out
}

func generateServerEntry(fields []string, result backend.ServerResult, console int) []byte {
	rec := result.Record
	if rec.SessionID == 0 && rec.PublicIP == "" {
		return nil
	}

	var flags byte
	flagsBuf := make([]byte, 0, 32)

	flags |= hasKeysFlag

	if rec.Natneg != "" {
		flags |= connectNegotiateFlag
	}

	publicIP, _ := strconv.ParseInt(rec.PublicIP, 10, 64)
	flagsBuf = append(flagsBuf, gamespy.GetBytesFromIntSigned(publicIP, console)...)

	flags |= nonstandardPortFlag
	port := rec.LocalPort
	if rec.PublicPort != "" && rec.PublicPort != "0" {
		port = rec.PublicPort
	}
	portNum, _ := strconv.Atoi(port)
	portBuf := make([]byte, 2)
	// Public/local ports in list entries are big-endian (Python: True).
	gamespy.WriteU16BE(portBuf, uint16(portNum))
	flagsBuf = append(flagsBuf, portBuf...)

	if rec.LocalIP0 != "" {
		flags |= privateIPFlag
		localIP := gamespy.ParseIPv4Bytes(rec.LocalIP0)
		flagsBuf = append(flagsBuf, localIP[0], localIP[1], localIP[2], localIP[3])
	}

	if rec.LocalPort != "" {
		flags |= nonstandardPrivatePortFlag
		localPort, _ := strconv.Atoi(rec.LocalPort)
		lpBuf := make([]byte, 2)
		gamespy.WriteU16BE(lpBuf, uint16(localPort))
		flagsBuf = append(flagsBuf, lpBuf...)
	}

	flags |= icmpIPFlag
	flagsBuf = append(flagsBuf, 0, 0, 0, 0)

	out := make([]byte, 0, 1+len(flagsBuf)+len(fields)*32)
	out = append(out, flags&0xff)
	out = append(out, flagsBuf...)

	if flags&hasKeysFlag != 0 {
		for _, field := range fields {
			out = append(out, 0xff)
			out = append(out, []byte(result.Requested[field])...)
			out = append(out, 0)
		}
	}

	return out
}

func (sess *connSession) sendEncrypted(gameName, challenge string, data []byte) {
	secret := sess.server.Keys[gameName]
	if secret == "" {
		logging.For("browser").Warnf("missing secret key for game %q", gameName)
	}
	enc := gamespy.EncTypeXEncrypt(secret, challenge, data)
	if enc != nil {
		_, _ = sess.conn.Write(enc)
	}
}

func (sess *connSession) handleSendMessage(packet []byte) {
	packetLen := int(gamespy.ReadU16BE(packet))
	if packetLen != len(packet) || len(packet) < 9 {
		logging.For("browser").Warnf("send message bad length from %s: header=%d actual=%d", sess.conn.RemoteAddr(), packetLen, len(packet))
		return
	}

	destIP := net.IPv4(packet[3], packet[4], packet[5], packet[6])
	destPort := int(binary.BigEndian.Uint16(packet[7:9]))
	payload := packet[9:]

	logging.For("browser").Infof("join relay %s -> %s:%d payload=%d bytes", sess.conn.RemoteAddr(), destIP.String(), destPort, len(payload))

	destAddr := net.UDPAddr{IP: destIP, Port: destPort}
	sess.forwardToClient(payload, destAddr)
}

func (sess *connSession) forwardToClient(data []byte, forwardClient net.UDPAddr) {
	server, ip := sess.findServerWithConsoleFallback(forwardClient.IP.String(), forwardClient.Port)
	if server == nil {
		logging.For("browser").Warnf("forwardToClient: no server for %s", forwardClient.String())
		return
	}

	if !gamespy.MatchPublicIP(server.PublicIP, ip) || server.PublicPort != strconv.Itoa(forwardClient.Port) {
		logging.For("browser").Warnf("forwardToClient: IP mismatch for %s (public=%s/%s)", forwardClient.String(), server.PublicIP, server.PublicPort)
		return
	}

	dest := forwardClient
	if forwardClient.Port == 0 && server.LocalPort != "" {
		localPort, _ := strconv.Atoi(server.LocalPort)
		dest.Port = localPort
	}

	var cookie uint32
	_ = binary.Read(rand.Reader, binary.LittleEndian, &cookie)

	sess.trackOwnServer(data)

	if len(data) == 10 && len(data) >= 6 && bytes.Equal(data[:6], natnegMagic) {
		natnegSession := int32(binary.LittleEndian.Uint32(data[6:10]))
		logging.For("browser").Debugf("natneg cookie session=%d for %s own=%v", natnegSession, forwardClient.String(), sess.ownServer != nil)
		sess.server.Backend.AddNatnegServer(uint32(natnegSession), server.AsMap())
		if sess.ownServer != nil {
			sess.server.Backend.AddNatnegServer(uint32(natnegSession), sess.ownServer.AsMap())
		}
	}

	if sess.server.Relay != nil {
		_ = sess.server.Relay.ForwardClientMessage(server.SessionID, cookie, dest, data)
	}
}

func (sess *connSession) trackOwnServer(data []byte) {
	if sess.ownServer != nil || len(data) < 16 {
		return
	}
	magic := data[:4]
	if !bytes.Equal(magic, sbcmMagic) && !bytes.Equal(magic, altMagic) {
		return
	}

	selfPort := int(gamespy.ReadU16LE(data[10:12]))
	selfIP := net.IPv4(data[12], data[13], data[14], data[15]).String()

	server, _ := sess.findServerWithConsoleFallback(selfIP, selfPort)
	sess.ownServer = server
}

func (sess *connSession) findServerWithConsoleFallback(dottedIP string, port int) (*backend.ServerRecord, string) {
	ip := net.ParseIP(dottedIP)
	if ip == nil {
		return nil, ""
	}

	first := sess.console
	if first != 0 && first != 1 {
		first = 0
	}
	for _, console := range []int{first, 1 - first} {
		signedIP := gamespy.SignedIPString(ip, console)
		if rec := sess.server.Backend.FindServerByAddress(signedIP, port, ""); rec != nil {
			return rec, signedIP
		}
	}

	if rec := sess.server.Backend.FindServerByAddress(dottedIP, port, ""); rec != nil {
		return rec, rec.PublicIP
	}
	return nil, ""
}

func shouldDropRequestedRoom(fields []string, requested map[string]string) bool {
	return len(fields) > 0 && requested != nil && len(requested) == 0
}

func isOnlySpace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
