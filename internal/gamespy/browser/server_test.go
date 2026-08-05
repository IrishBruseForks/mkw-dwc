package browser

import (
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

func TestDispatchPacketLengthBigEndian(t *testing.T) {
	filter := "dwc_mver = 90"
	fields := "dwc_pid"
	body := []byte{
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
	}
	body = append(body, []byte("mariokartwii")...)
	body = append(body, 0)
	body = append(body, []byte("mariokartwii")...)
	body = append(body, 0)
	body = append(body, []byte("12345678")...)
	body = append(body, []byte(filter)...)
	body = append(body, 0)
	body = append(body, []byte(fields)...)
	body = append(body, 0)
	body = append(body, 0, 0, 0, 0)

	packet := make([]byte, 3+len(body))
	binary.BigEndian.PutUint16(packet[0:2], uint16(len(packet)))
	packet[2] = cmdServerList
	copy(packet[3:], body)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, client)
		close(done)
	}()

	sess := &connSession{
		conn: server,
		server: &Server{
			Backend: backend.New(),
			Keys:    gamespy.SecretKeys(),
		},
	}

	if !sess.dispatch(packet) {
		t.Fatal("dispatch returned false")
	}
	if len(sess.buffer) != 0 {
		t.Fatalf("expected full packet consumed, %d bytes remaining", len(sess.buffer))
	}
	_ = server.Close()
	<-done
}

func TestParseServerListOptionsBigEndianLimit(t *testing.T) {
	// Fixture mirrors live MKWii: options=0x00000080 (BE) + maxServers=6 (LE).
	filter := "dwc_pid = 4"
	fields := `\numplayers\maxplayers\dwc_pid`
	body := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00}
	body = append(body, []byte("mariokartwii")...)
	body = append(body, 0)
	body = append(body, []byte("mariokartwii")...)
	body = append(body, 0)
	body = append(body, []byte("MF?Tt7S/")...)
	body = append(body, []byte(filter)...)
	body = append(body, 0)
	body = append(body, []byte(fields)...)
	body = append(body, 0)
	body = append(body, 0x00, 0x00, 0x00, 0x80) // options BE
	body = append(body, 0x06, 0x00, 0x00, 0x00) // maxServers LE

	packet := make([]byte, 3+len(body))
	binary.BigEndian.PutUint16(packet[0:2], uint16(len(packet)))
	packet[2] = cmdServerList
	copy(packet[3:], body)

	b := backend.New()
	_ = b.UpdateServerList("mariokartwii", 0x11111111, map[string]string{
		"dwc_pid":    "4",
		"publicip":   "2130706433",
		"publicport": "12345",
		"localport":  "12345",
		"localip0":   "10.0.1.30",
		"natneg":     "1",
	}, 1)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := client.Read(buf)
		readDone <- append([]byte(nil), buf[:n]...)
	}()

	sess := &connSession{
		conn: server,
		server: &Server{
			Backend: b,
			Keys:    gamespy.SecretKeys(),
		},
	}
	if !sess.dispatch(packet) {
		t.Fatal("dispatch returned false")
	}

	enc := <-readDone
	if len(enc) == 0 {
		t.Fatal("expected encrypted server list reply")
	}
}

func TestGenerateServerListHeaderPortBigEndian(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// net.Pipe remote addr is not TCP; inject via a fake Addr by wrapping is hard.
	// Instead validate the BE helper encoding used by the header writer.
	portBuf := make([]byte, 2)
	gamespy.WriteU16BE(portBuf, 52554)
	if portBuf[0] != 0xcd || portBuf[1] != 0x4a {
		t.Fatalf("expected BE 52554 == cd4a, got %x", portBuf)
	}
	gamespy.WriteU16LE(portBuf, 14)
	if portBuf[0] != 0x0e || portBuf[1] != 0x00 {
		t.Fatalf("expected LE key count 14 == 0e00, got %x", portBuf)
	}
}

func TestGenerateServerEntryPortsBigEndian(t *testing.T) {
	entry := generateServerEntry([]string{"dwc_pid"}, backend.ServerResult{
		Record: backend.ServerRecord{
			SessionID:  1,
			PublicIP:   "2130706433",
			PublicPort: "50938",
			LocalPort:  "50938",
			LocalIP0:   "10.0.1.30",
			Natneg:     "1",
			Console:    1,
		},
		Requested: map[string]string{"dwc_pid": "4"},
	}, 1)
	if len(entry) < 7 {
		t.Fatalf("entry too short: %d", len(entry))
	}
	// flags + publicip(4) + publicport(2 BE)
	// After flags byte and 4-byte IP, next 2 bytes are public port BE.
	portOff := 1 + 4
	got := binary.BigEndian.Uint16(entry[portOff : portOff+2])
	if got != 50938 {
		t.Fatalf("public port BE: got %d want 50938 (bytes %x)", got, entry[portOff:portOff+2])
	}
}

func TestSendMessagePacketLengthBigEndian(t *testing.T) {
	payload := []byte{0x53, 0x42, 0x43, 0x4d}
	packet := make([]byte, 9+len(payload))
	binary.BigEndian.PutUint16(packet[0:2], uint16(len(packet)))
	packet[2] = cmdSendMsg
	packet[3], packet[4], packet[5], packet[6] = 127, 0, 0, 1
	binary.BigEndian.PutUint16(packet[7:9], 27900)
	copy(packet[9:], payload)

	if int(gamespy.ReadU16BE(packet)) != len(packet) {
		t.Fatalf("fixture packet length mismatch")
	}
}

func TestShouldDropRequestedRoom(t *testing.T) {
	requestedWithEmptyValues := map[string]string{"dwc_pid": "", "rk": ""}
	if shouldDropRequestedRoom([]string{"dwc_pid", "rk"}, requestedWithEmptyValues) {
		t.Fatal("requested with keys and empty values must not be dropped")
	}

	if !shouldDropRequestedRoom([]string{"dwc_pid"}, map[string]string{}) {
		t.Fatal("empty requested map with fields must be dropped")
	}

	if shouldDropRequestedRoom(nil, map[string]string{}) {
		t.Fatal("nil fields must not trigger drop")
	}

	if shouldDropRequestedRoom([]string{"dwc_pid"}, nil) {
		t.Fatal("nil requested must not trigger drop")
	}
}

func TestFindServerWithConsoleFallbackPrefersSessionConsole(t *testing.T) {
	logging.Init(logging.Settings{Level: "error", Color: "never", Timestamps: false})

	dotted := "192.168.1.1"
	ip := net.ParseIP(dotted)
	wiiSigned := gamespy.SignedIPString(ip, 1)
	dsSigned := gamespy.SignedIPString(ip, 0)
	if wiiSigned == dsSigned {
		t.Fatal("fixture IP must produce different signed forms per console")
	}

	b := backend.New()
	_ = b.UpdateServerList("mariokartwii", 0x01020304, map[string]string{
		"publicip":   wiiSigned,
		"publicport": "27900",
	}, 1)

	sess := &connSession{
		server:  &Server{Backend: b},
		console: 1,
	}
	rec, signedIP := sess.findServerWithConsoleFallback(dotted, 27900)
	if rec == nil {
		t.Fatal("expected server lookup to succeed")
	}
	if signedIP != wiiSigned {
		t.Fatalf("expected Wii signed IP %q, got %q", wiiSigned, signedIP)
	}

	sess.console = 0
	_, signedIP = sess.findServerWithConsoleFallback(dotted, 27900)
	if signedIP != dsSigned {
		t.Fatalf("expected DS signed IP %q with console=0, got %q", dsSigned, signedIP)
	}
}
