package qr_test

import (
	"context"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/qr"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

func TestHeartbeatReturnsChallenge(t *testing.T) {
	logging.Init(logging.Settings{Level: "error", Color: "never", Timestamps: false})

	port := freeUDPPort(t)
	be := backend.New()
	srv := qr.New(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), be, gamespy.SecretKeys())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx)

	time.Sleep(100 * time.Millisecond)

	conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	sessionID := uint32(0x12345678)
	kv := []string{
		"gamename", "mariokartwii",
		"dwc_pid", "1",
		"publicip", "0",
		"publicport", "27900",
		"localport", "27900",
	}
	payload := strings.Join(kv, "\x00") + "\x00"
	packet := make([]byte, 5+len(payload))
	packet[0] = 0x03
	binary.LittleEndian.PutUint32(packet[1:], sessionID)
	copy(packet[5:], payload)

	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n < 3 || buf[2] != 0x01 {
		t.Fatalf("expected challenge cmd 0x01, got % x", buf[:n])
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free udp: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}
