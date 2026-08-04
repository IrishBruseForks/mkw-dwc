package natneg_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/natneg"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

func TestBackupTestEchoesBackupAck(t *testing.T) {
	logging.Init(logging.Settings{Level: "error", Color: "never", Timestamps: false})

	port := freeUDPPort(t)
	srv := natneg.New(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), backend.New())

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

	packet := []byte{
		0xfd, 0xfc, 0x1e, 0x66, 0x6a, 0xb2,
		0x03, 0x08,
		0x01, 0x02, 0x03, 0x04,
		0x00, 0x00,
	}
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n < 8 || buf[7] != 0x09 {
		t.Fatalf("expected BACKUP_ACK (0x09), got % x", buf[:n])
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
