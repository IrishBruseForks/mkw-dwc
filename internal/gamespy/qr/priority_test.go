package qr

import (
	"net"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

type mockPacketConn struct {
	writes int
}

func (m *mockPacketConn) ReadFrom(p []byte) (int, net.Addr, error) { return 0, nil, nil }
func (m *mockPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	m.writes++
	return len(p), nil
}
func (m *mockPacketConn) Close() error                       { return nil }
func (m *mockPacketConn) LocalAddr() net.Addr                { return nil }
func (m *mockPacketConn) SetDeadline(time.Time) error        { return nil }
func (m *mockPacketConn) SetReadDeadline(time.Time) error    { return nil }
func (m *mockPacketConn) SetWriteDeadline(time.Time) error   { return nil }

func TestQueueWritePriorityBypassesFullQueue(t *testing.T) {
	logging.Init(logging.Settings{Level: "error", Color: "never", Timestamps: false})

	mock := &mockPacketConn{}
	s := &Server{
		writeCh: make(chan writeItem, 1),
		conn:    mock,
	}
	s.writeCh <- writeItem{data: []byte("blocked"), addr: net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1}}

	addr := net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 27900}
	s.queueWritePriority([]byte{0xfe, 0xfd, 0x01}, addr, true)

	if mock.writes != 1 {
		t.Fatalf("priority write should bypass full queue, got %d writes", mock.writes)
	}
}
