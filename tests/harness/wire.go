package harness

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
)

// GameSpyField reads a key from a GameSpy wire message.
func GameSpyField(msg, key string) string {
	needle := "\\" + key + "\\"
	idx := strings.Index(msg, needle)
	if idx < 0 {
		return ""
	}
	rest := msg[idx+len(needle):]
	end := strings.Index(rest, "\\")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// BuildQRHeartbeat builds a QR heartbeat (cmd 0x03) packet.
func BuildQRHeartbeat(sessionID uint32, fields map[string]string) []byte {
	var kv []string
	for k, v := range fields {
		kv = append(kv, k, v)
	}
	payload := strings.Join(kv, "\x00") + "\x00"
	packet := make([]byte, 5+len(payload))
	packet[0] = 0x03
	binary.LittleEndian.PutUint32(packet[1:], sessionID)
	copy(packet[5:], payload)
	return packet
}

// BuildQRChallengeResponse builds a QR challenge response (cmd 0x01).
func BuildQRChallengeResponse(sessionID uint32, proof string) []byte {
	packet := make([]byte, 5+len(proof)+1)
	packet[0] = 0x01
	binary.LittleEndian.PutUint32(packet[1:], sessionID)
	copy(packet[5:], proof)
	return packet
}

// BuildBrowserServerListPacket builds a browser server-list request (cmd 0x00).
func BuildBrowserServerListPacket(gameName, filter, fields, challenge string) []byte {
	body := []byte{0x00, 0x01}
	body = append(body, 0, 0, 0, 0)
	body = append(body, []byte(gameName)...)
	body = append(body, 0)
	body = append(body, []byte(gameName)...)
	body = append(body, 0)
	body = append(body, []byte(challenge)...)
	body = append(body, []byte(filter)...)
	body = append(body, 0)
	body = append(body, []byte(fields)...)
	body = append(body, 0)
	body = append(body, 0, 0, 0, 0)

	packet := make([]byte, 3+len(body))
	binary.BigEndian.PutUint16(packet[0:2], uint16(len(packet)))
	packet[2] = 0x00
	copy(packet[3:], body)
	return packet
}

// BuildBrowserSendMessagePacket builds a browser send-message request (cmd 0x02).
func BuildBrowserSendMessagePacket(destIP net.IP, destPort int, payload []byte) []byte {
	ip4 := destIP.To4()
	packet := make([]byte, 9+len(payload))
	binary.BigEndian.PutUint16(packet[0:2], uint16(len(packet)))
	packet[2] = 0x02
	copy(packet[3:7], ip4)
	binary.BigEndian.PutUint16(packet[7:9], uint16(destPort))
	copy(packet[9:], payload)
	return packet
}

// BuildBrowserKeepAlivePacket builds a browser keep-alive (cmd 0x03).
func BuildBrowserKeepAlivePacket() []byte {
	return []byte{0x00, 0x03, 0x03}
}

// BuildNatnegInit builds a NATNEG INIT packet.
func BuildNatnegInit(cookie uint32, clientIndex byte, gameName string) []byte {
	packet := []byte{
		0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2,
		0x03, 0x00,
	}
	sess := make([]byte, 4)
	binary.LittleEndian.PutUint32(sess, cookie)
	packet = append(packet, sess...)
	packet = append(packet, 0x00, clientIndex, 0x01)
	packet = append(packet, 0x0a, 0x00, 0x01, 0xe2, 0x00, 0x00)
	packet = append(packet, []byte(gameName)...)
	packet = append(packet, 0x00)
	return packet
}

// BuildNatnegAddressCheck builds a minimal NATNEG ADDRESS_CHECK packet.
func BuildNatnegAddressCheck(cookie uint32, clientIndex byte) []byte {
	packet := BuildNatnegInit(cookie, clientIndex, "mariokartwii")
	packet[7] = 0x0a
	return packet
}

// BuildNatnegNatify builds a NATNEG NATIFY packet.
func BuildNatnegNatify(cookie uint32) []byte {
	packet := BuildNatnegInit(cookie, 0, "mariokartwii")
	packet[7] = 0x0c
	return packet
}

// BuildNatnegReport builds a NATNEG REPORT packet.
func BuildNatnegReport(cookie uint32) []byte {
	packet := make([]byte, 21)
	copy(packet, BuildNatnegInit(cookie, 0, "mariokartwii")[:21])
	packet[7] = 0x0d
	return packet
}

// ReadUDP reads one UDP datagram.
func ReadUDP(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("udp read: %v", err)
	}
	return buf[:n]
}

// DialUDP dials UDP with a deadline.
func DialUDP(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("udp dial %s: %v", addr, err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// QRRegisterRoom performs heartbeat + challenge and returns the session id.
func QRRegisterRoom(t *testing.T, qrAddr string, profileID int64, sessionID uint32) {
	t.Helper()
	conn := DialUDP(t, qrAddr)
	defer conn.Close()

	heartbeat := BuildQRHeartbeat(sessionID, map[string]string{
		"gamename":      "mariokartwii",
		"dwc_pid":       strconv.FormatInt(profileID, 10),
		"publicip":      "0",
		"publicport":    "27900",
		"localport":     "27900",
		"natneg":        "1",
		"statechanged":  "1",
		"maxplayers":    "11",
		"numplayers":    "1",
		"dwc_mver":      "90",
		"dwc_mtype":     "0",
		"dwc_hoststate": "2",
		"dwc_suspend":   "0",
		"rk":            "vs_123",
		"ev":            "5000",
		"p":             "0",
	})
	if _, err := conn.Write(heartbeat); err != nil {
		t.Fatalf("qr heartbeat: %v", err)
	}

	challengePacket := ReadUDP(t, conn)
	if len(challengePacket) < 7 || challengePacket[2] != 0x01 {
		t.Fatalf("expected qr challenge, got % x", challengePacket)
	}
	challenge := string(bytes.TrimRight(challengePacket[7:], "\x00"))

	proof := gamespy.PrepareRC4Base64("9r3Rmy", challenge)
	if _, err := conn.Write(BuildQRChallengeResponse(sessionID, proof)); err != nil {
		t.Fatalf("qr challenge response: %v", err)
	}

	regPacket := ReadUDP(t, conn)
	if len(regPacket) < 3 || regPacket[2] != 0x0a {
		t.Fatalf("expected qr registered, got % x", regPacket)
	}
}
