package qr_test

import (
	"bytes"
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

func TestChallengeResponseStripsTrailingByte(t *testing.T) {
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
	heartbeat := make([]byte, 5+len(payload))
	heartbeat[0] = 0x03
	binary.LittleEndian.PutUint32(heartbeat[1:], sessionID)
	copy(heartbeat[5:], payload)

	if _, err := conn.Write(heartbeat); err != nil {
		t.Fatalf("heartbeat write: %v", err)
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("challenge read: %v", err)
	}
	if n < 7 || buf[2] != 0x01 {
		t.Fatalf("expected challenge cmd 0x01, got % x", buf[:n])
	}
	challenge := string(bytes.TrimRight(buf[7:n], "\x00"))
	proof := gamespy.PrepareRC4Base64("9r3Rmy", challenge)

	// Clients append a trailing byte after the proof; Python uses recv_data[5:-1].
	const trailer = byte(0xff)
	response := make([]byte, 5+len(proof)+1)
	response[0] = 0x01
	binary.LittleEndian.PutUint32(response[1:], sessionID)
	copy(response[5:], proof)
	response[5+len(proof)] = trailer

	if _, err := conn.Write(response); err != nil {
		t.Fatalf("challenge response write: %v", err)
	}

	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("registered read: %v", err)
	}
	if n < 3 || buf[2] != 0x0a {
		t.Fatalf("expected registered cmd 0x0a, got % x", buf[:n])
	}

	// Same payload would fail if the trailer were kept in the comparison.
	if string(response[5:]) == proof {
		t.Fatal("test setup error: trailer must not be part of proof")
	}
}

func TestChallengeResponseWithoutTrailerFails(t *testing.T) {
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

	sessionID := uint32(0x87654321)
	kv := []string{
		"gamename", "mariokartwii",
		"dwc_pid", "2",
		"publicip", "0",
		"publicport", "27900",
		"localport", "27900",
	}
	payload := strings.Join(kv, "\x00") + "\x00"
	heartbeat := make([]byte, 5+len(payload))
	heartbeat[0] = 0x03
	binary.LittleEndian.PutUint32(heartbeat[1:], sessionID)
	copy(heartbeat[5:], payload)

	if _, err := conn.Write(heartbeat); err != nil {
		t.Fatalf("heartbeat write: %v", err)
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("challenge read: %v", err)
	}
	challenge := string(bytes.TrimRight(buf[7:n], "\x00"))
	proof := gamespy.PrepareRC4Base64("9r3Rmy", challenge)

	// No trailing byte: Python recv_data[5:-1] would drop the last proof character.
	response := make([]byte, 5+len(proof))
	response[0] = 0x01
	binary.LittleEndian.PutUint32(response[1:], sessionID)
	copy(response[5:], proof)

	if _, err := conn.Write(response); err != nil {
		t.Fatalf("challenge response write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected no registered response without trailing byte")
	}

	rooms, err := be.FindServers("mariokartwii", "dwc_pid = 2", []string{"dwc_pid"}, 0)
	if err != nil {
		t.Fatalf("FindServers: %v", err)
	}
	if len(rooms) != 0 {
		t.Fatalf("expected room not registered, got %d", len(rooms))
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
