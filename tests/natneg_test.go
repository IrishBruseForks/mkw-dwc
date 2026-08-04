package tests_test

import (
	"net"
	"testing"

	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestNatnegReferenceHandlers(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	hostConn := harness.DialUDP(t, env.NatNegAddr())
	defer hostConn.Close()
	clientConn := harness.DialUDP(t, env.NatNegAddr())
	defer clientConn.Close()

	cookie := uint32(0x71f1003d)
	hostInit := harness.BuildNatnegInit(cookie, 0x00, "mariokartwii")
	clientInit := harness.BuildNatnegInit(cookie, 0x01, "mariokartwii")

	if _, err := hostConn.Write(hostInit); err != nil {
		t.Fatalf("host init: %v", err)
	}
	initAck := harness.ReadUDP(t, hostConn)
	if len(initAck) < 8 || initAck[7] != 0x01 {
		t.Fatalf("expected INIT_ACK, got % x", initAck)
	}

	if _, err := clientConn.Write(clientInit); err != nil {
		t.Fatalf("client init: %v", err)
	}
	initAck = harness.ReadUDP(t, clientConn)
	if len(initAck) < 8 || initAck[7] != 0x01 {
		t.Fatalf("expected client INIT_ACK, got % x", initAck)
	}

	connectHost := harness.ReadUDP(t, hostConn)
	connectClient := harness.ReadUDP(t, clientConn)
	if len(connectHost) < 8 || connectHost[7] != 0x05 {
		t.Fatalf("host expected CONNECT, got % x", connectHost)
	}
	if len(connectClient) < 8 || connectClient[7] != 0x05 {
		t.Fatalf("client expected CONNECT, got % x", connectClient)
	}

	addrCheck := harness.BuildNatnegAddressCheck(cookie, 0)
	if _, err := hostConn.Write(addrCheck); err != nil {
		t.Fatalf("address check: %v", err)
	}
	addrReply := harness.ReadUDP(t, hostConn)
	if len(addrReply) < 8 || addrReply[7] != 0x0b {
		t.Fatalf("expected ADDRESS_REPLY, got % x", addrReply)
	}

	natify := harness.BuildNatnegNatify(cookie)
	if _, err := hostConn.Write(natify); err != nil {
		t.Fatalf("natify: %v", err)
	}
	ert := harness.ReadUDP(t, hostConn)
	if len(ert) < 8 || ert[7] != 0x02 {
		t.Fatalf("expected ERT_TEST, got % x", ert)
	}

	report := harness.BuildNatnegReport(cookie)
	if _, err := hostConn.Write(report); err != nil {
		t.Fatalf("report: %v", err)
	}
	reportAck := harness.ReadUDP(t, hostConn)
	if len(reportAck) < 8 || reportAck[7] != 0x0e {
		t.Fatalf("expected REPORT_ACK, got % x", reportAck)
	}

	backup := append([]byte(nil), hostInit...)
	backup[7] = 0x08
	if _, err := hostConn.Write(backup); err != nil {
		t.Fatalf("backup test: %v", err)
	}
	backupAck := harness.ReadUDP(t, hostConn)
	if len(backupAck) < 8 || backupAck[7] != 0x09 {
		t.Fatalf("expected BACKUP_ACK, got % x", backupAck)
	}
}

func TestNatnegAddressReplyContainsClientIP(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	conn := harness.DialUDP(t, env.NatNegAddr())
	defer conn.Close()

	cookie := uint32(1)
	packet := harness.BuildNatnegAddressCheck(cookie, 0)
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("write: %v", err)
	}
	reply := harness.ReadUDP(t, conn)
	if len(reply) < 21 {
		t.Fatalf("short address reply: %d", len(reply))
	}
	ip := net.IPv4(reply[15], reply[16], reply[17], reply[18])
	if !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("address reply ip=%v", ip)
	}
}
