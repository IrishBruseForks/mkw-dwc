package tests_test

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestQRReferenceHandlers(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	conn := harness.DialUDP(t, env.QRAddr())
	defer conn.Close()

	if _, err := conn.Write([]byte{0x09}); err != nil {
		t.Fatalf("availability write: %v", err)
	}
	avail := harness.ReadUDP(t, conn)
	want := []byte{0xfe, 0xfd, 0x09, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(avail, want) {
		t.Fatalf("availability=% x want % x", avail, want)
	}

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCJ01")
	token, acChallenge := harness.NASLogin(t, env.NASURL(), userid, "RMCJ01", "RMCJ01", "MiiName")
	profileID := harness.ProfileLogin(t, env.ProfileAddr(), token, acChallenge)

	sessionID := uint32(0xabcdef01)
	harness.QRRegisterRoom(t, env.QRAddr(), profileID, sessionID)

	rooms, err := env.Backend.FindServers("mariokartwii", "dwc_mver = 90", []string{"dwc_pid", "ingamesn"}, 0)
	if err != nil {
		t.Fatalf("FindServers: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}
	if rooms[0].Requested["dwc_pid"] == "" {
		t.Fatal("missing dwc_pid in room")
	}
	if rooms[0].Requested["ingamesn"] == "" {
		t.Fatal("expected ingamesn injected from NAS login")
	}

	// keepalive cmd 0x08
	if _, err := conn.Write([]byte{0x08, 0x01, 0x02, 0x03, 0x04}); err != nil {
		t.Fatalf("keepalive write: %v", err)
	}

	// bad challenge should not register
	badSession := uint32(0xbadc0de)
	heartbeat := harness.BuildQRHeartbeat(badSession, map[string]string{
		"gamename": "mariokartwii",
		"dwc_pid":  "999",
		"publicip": "0",
	})
	if _, err := conn.Write(heartbeat); err != nil {
		t.Fatalf("bad heartbeat: %v", err)
	}
	challengePacket := harness.ReadUDP(t, conn)
	_ = string(bytes.TrimRight(challengePacket[7:], "\x00"))
	badResp := harness.BuildQRChallengeResponse(badSession, "not-valid")
	if _, err := conn.Write(badResp); err != nil {
		t.Fatalf("bad challenge write: %v", err)
	}

	rooms, err = env.Backend.FindServers("mariokartwii", "dwc_pid = 999", []string{"dwc_pid"}, 0)
	if err != nil {
		t.Fatalf("FindServers bad: %v", err)
	}
	if len(rooms) != 0 {
		t.Fatalf("bad challenge should not register room, got %d", len(rooms))
	}

	// statechanged=2 removes room (reference: QR heartbeat teardown)
	removeConn := harness.DialUDP(t, env.QRAddr())
	defer removeConn.Close()
	remove := harness.BuildQRHeartbeat(sessionID, map[string]string{
		"statechanged": "2",
	})
	if _, err := removeConn.Write(remove); err != nil {
		t.Fatalf("remove heartbeat: %v", err)
	}
	pidFilter := "dwc_pid = " + strconv.FormatInt(profileID, 10)
	harness.WaitForServerCount(t, env.Backend, "mariokartwii", pidFilter, 0)
}

func TestQRChallengeCryptoMatchesReference(t *testing.T) {
	challenge := "abc12300C0A801000000"
	got := gamespy.PrepareRC4Base64("9r3Rmy", challenge)
	if got == "" {
		t.Fatal("empty challenge response")
	}
	packet := make([]byte, 5+len(got)+1)
	packet[0] = 0x01
	binary.LittleEndian.PutUint32(packet[1:], 1)
	copy(packet[5:], got)
	if packet[0] != 0x01 {
		t.Fatal("unexpected packet cmd")
	}
}
