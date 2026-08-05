package tests_test

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/database"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestGPSPOthersListJSON(t *testing.T) {
	runGPSPOthersList(t, openJSONStore(t))
}

func runGPSPOthersList(t *testing.T, gpcm database.Store) {
	t.Helper()
	env := harness.Start(t, gpcm)

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCE01")
	token, acChallenge := harness.NASLogin(t, env.NASURL(), userid, "RMCE01", "RMCE01", "")

	profileID := harness.ProfileLogin(t, env.ProfileAddr(), token, acChallenge)
	sesskey, uniquenick := profileLoginSesskey(t, env.ProfileAddr(), token, acChallenge)

	pidStr := strconv.FormatInt(profileID, 10)
	reply := harness.GPSPOthersList(t, env.GPSPAddr(), pidStr, sesskey, pidStr)

	if !strings.Contains(reply, `\otherslist\`) {
		t.Fatalf("missing otherslist in %q", reply)
	}
	if !strings.Contains(reply, `\o\`+pidStr+`\`) {
		t.Fatalf("missing o\\%s\\ in %q", pidStr, reply)
	}
	if !strings.Contains(reply, `\uniquenick\`+uniquenick+`\`) {
		t.Fatalf("missing uniquenick %q in %q", uniquenick, reply)
	}
	if !strings.Contains(reply, `\oldone\`) {
		t.Fatalf("missing oldone in %q", reply)
	}

	reply0 := harness.GPSPOthersList(t, env.GPSPAddr(), pidStr, sesskey, "0")
	if !strings.Contains(reply0, `\oldone\`) {
		t.Fatalf("opids=0 missing oldone in %q", reply0)
	}
}

func profileLoginSesskey(t *testing.T, addr, authtoken, acChallenge string) (sesskey, uniquenick string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("profile dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("profile lc read: %v", err)
	}
	serverChallenge := harness.GameSpyField(string(buf[:n]), "challenge")

	clientChallenge := "CLIENTCHAL"
	response := gamespy.GenerateResponse(serverChallenge, acChallenge, clientChallenge, authtoken)
	loginMsg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "login",
		"__cmd_val__": "",
		"authtoken":   authtoken,
		"challenge":   clientChallenge,
		"response":    response,
		"id":          "1",
	})
	if _, err := conn.Write(loginMsg); err != nil {
		t.Fatalf("profile login write: %v", err)
	}

	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("profile login read: %v", err)
	}
	msg := string(buf[:n])
	if strings.Contains(msg, `\error\`) {
		t.Fatalf("profile login error: %q", msg)
	}
	sesskey = harness.GameSpyField(msg, "sesskey")
	uniquenick = harness.GameSpyField(msg, "uniquenick")
	if sesskey == "" || uniquenick == "" {
		t.Fatalf("profile login incomplete: %q", msg)
	}
	return sesskey, uniquenick
}
