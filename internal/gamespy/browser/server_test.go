package browser

import (
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
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
