package tests_test

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestProfileReferenceHandlers(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCE01")
	token, acChallenge := harness.NASLogin(t, env.NASURL(), userid, "RMCE01", "RMCE01", "")

	profileID := harness.ProfileLogin(t, env.ProfileAddr(), token, acChallenge)
	if profileID != 1 {
		t.Fatalf("profileid=%d want 1", profileID)
	}

	harness.ProfileKA(t, env.ProfileAddr())

	conn, profileID2, sesskey := harness.ProfileSession(t, env.ProfileAddr(), token, acChallenge)
	defer conn.Close()
	if profileID2 != profileID {
		t.Fatalf("session profileid=%d want %d", profileID2, profileID)
	}

	update := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "updatepro",
		"__cmd_val__": "",
		"sesskey":     sesskey,
		"firstname":   "Wii:2555151656076614@WR9E",
		"partnerid":   "11",
	})
	if _, err := conn.Write(update); err != nil {
		t.Fatalf("updatepro write: %v", err)
	}

	get := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "getprofile",
		"__cmd_val__": "",
		"sesskey":     sesskey,
		"profileid":   strconv.FormatInt(profileID, 10),
		"id":          "2",
	})
	if _, err := conn.Write(get); err != nil {
		t.Fatalf("getprofile write: %v", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("getprofile read: %v", err)
	}
	msg := string(buf[:n])
	if !strings.Contains(msg, `\pi\`) {
		t.Fatalf("expected pi reply, got %q", msg)
	}
	if harness.GameSpyField(msg, "profileid") != strconv.FormatInt(profileID, 10) {
		t.Fatalf("pi profileid missing: %q", msg)
	}
	if harness.GameSpyField(msg, "firstname") != "Wii:2555151656076614@WR9E" {
		t.Fatalf("pi firstname missing after updatepro: %q", msg)
	}
	if harness.GameSpyField(msg, "sig") == "" || harness.GameSpyField(msg, "uniquenick") == "" {
		t.Fatalf("pi incomplete: %q", msg)
	}

	status := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "status",
		"__cmd_val__": "1",
		"sesskey":     sesskey,
		"statstring":  "/SCM/0/SCN/0/VER/90",
		"locstring":   "",
	})
	if _, err := conn.Write(status); err != nil {
		t.Fatalf("status write: %v", err)
	}

	badConn, err := net.Dial("tcp", env.ProfileAddr())
	if err != nil {
		t.Fatalf("profile dial: %v", err)
	}
	defer badConn.Close()

	if _, err := badConn.Read(buf); err != nil {
		t.Fatalf("lc read: %v", err)
	}

	badLogin := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "login",
		"__cmd_val__": "",
		"authtoken":   token,
		"challenge":   "BADCHALL",
		"response":    "deadbeef",
		"id":          "1",
	})
	if _, err := badConn.Write(badLogin); err != nil {
		t.Fatalf("bad login write: %v", err)
	}
	n, err = badConn.Read(buf)
	if err != nil {
		t.Fatalf("bad login read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), `\err\256\`) {
		t.Fatalf("expected login error 256, got %q", buf[:n])
	}
}
