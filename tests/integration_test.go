package tests_test

import (
	"net"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

const mkwiiFilter = "dwc_mver = 90 and dwc_pid != 1 and maxplayers = 11 and numplayers < 11 and dwc_mtype = 0 and dwc_hoststate = 2 and dwc_suspend = 0 and (rk = 'vs_123' and (ev > 4263 or ev <= 5763) and p = 0)"

func TestBackendMKWFilterMatchesReference(t *testing.T) {
	be := backend.New()
	server := map[string]string{
		"dwc_mver": "90", "dwc_pid": "2", "maxplayers": "11", "numplayers": "10",
		"dwc_mtype": "0", "dwc_hoststate": "2", "dwc_suspend": "0",
		"rk": "vs_123", "ev": "5000", "p": "0",
	}
	_ = be.UpdateServerList("mariokartwii", 1, server, 1)

	ok, err := backend.MatchFilter(server, mkwiiFilter)
	if err != nil || !ok {
		t.Fatalf("MatchFilter: ok=%v err=%v", ok, err)
	}

	results, err := be.FindServers("mariokartwii", mkwiiFilter, []string{"dwc_pid", "ev", "rk"}, 0)
	if err != nil {
		t.Fatalf("FindServers: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Requested["rk"] != "vs_123" {
		t.Fatalf("rk=%q", results[0].Requested["rk"])
	}
}

func TestCoreProtocolFlowJSON(t *testing.T) {
	runCoreProtocolFlow(t, openJSONStore(t))
}

func runCoreProtocolFlow(t *testing.T, gpcm database.Store) {
	t.Helper()
	env := harness.Start(t, gpcm)

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCJ01")
	token, acChallenge := harness.NASLogin(t, env.NASURL(), userid, "RMCJ01", "RMCJ01", "TestMii")
	profileID := harness.ProfileLogin(t, env.ProfileAddr(), token, acChallenge)

	sessionID := uint32(0x12345678)
	harness.QRRegisterRoom(t, env.QRAddr(), profileID, sessionID)

	conn, err := net.Dial("tcp", env.BrowserAddr())
	if err != nil {
		t.Fatalf("browser dial: %v", err)
	}
	defer conn.Close()
	list := harness.BuildBrowserServerListPacket("mariokartwii", "dwc_mver = 90", "dwc_pid", "12345678")
	if _, err := conn.Write(list); err != nil {
		t.Fatalf("browser write: %v", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil || n < 20 {
		t.Fatalf("browser read: %v n=%d", err, n)
	}

	hostConn := harness.DialUDP(t, env.NatNegAddr())
	defer hostConn.Close()
	clientConn := harness.DialUDP(t, env.NatNegAddr())
	defer clientConn.Close()

	cookie := sessionID
	if _, err := hostConn.Write(harness.BuildNatnegInit(cookie, 0x00, "mariokartwii")); err != nil {
		t.Fatalf("natneg host init: %v", err)
	}
	harness.ReadUDP(t, hostConn)
	if _, err := clientConn.Write(harness.BuildNatnegInit(cookie, 0x01, "mariokartwii")); err != nil {
		t.Fatalf("natneg client init: %v", err)
	}
	harness.ReadUDP(t, clientConn)
	if pkt := harness.ReadUDP(t, hostConn); len(pkt) < 8 || pkt[7] != 0x05 {
		t.Fatalf("host CONNECT missing: % x", pkt)
	}
	if pkt := harness.ReadUDP(t, clientConn); len(pkt) < 8 || pkt[7] != 0x05 {
		t.Fatalf("client CONNECT missing: % x", pkt)
	}
}
