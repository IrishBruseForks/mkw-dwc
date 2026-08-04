package tests_test

import (
	"net"
	"testing"

	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

const mkwiiBrowserFilter = "dwc_mver = 90"

func TestBrowserReferenceHandlers(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCJ01")
	token, acChallenge := harness.NASLogin(t, env.NASURL(), userid, "RMCJ01", "RMCJ01", "")
	profileID := harness.ProfileLogin(t, env.ProfileAddr(), token, acChallenge)

	sessionID := uint32(0x12345678)
	harness.QRRegisterRoom(t, env.QRAddr(), profileID, sessionID)

	conn, err := net.Dial("tcp", env.BrowserAddr())
	if err != nil {
		t.Fatalf("browser dial: %v", err)
	}
	defer conn.Close()

	// empty filter triggers own-ip EncTypeX response (Python reference behavior)
	ownIP := harness.BuildBrowserServerListPacket("mariokartwii", "", "", "12345678")
	if _, err := conn.Write(ownIP); err != nil {
		t.Fatalf("own-ip write: %v", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("own-ip read: %v", err)
	}
	if n < 20 {
		t.Fatalf("own-ip response too short: %d", n)
	}

	// MKW filter should return encrypted server list
	list := harness.BuildBrowserServerListPacket("mariokartwii", mkwiiBrowserFilter, "dwc_pid", "12345678")
	if _, err := conn.Write(list); err != nil {
		t.Fatalf("server list write: %v", err)
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("server list read: %v", err)
	}
	if n < 20 {
		t.Fatalf("server list response too short: %d", n)
	}

	// keepalive cmd 0x03 should not close connection
	keepalive := harness.BuildBrowserKeepAlivePacket()
	if _, err := conn.Write(keepalive); err != nil {
		t.Fatalf("keepalive write: %v", err)
	}
}
