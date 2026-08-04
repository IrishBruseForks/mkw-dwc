package tests_test

import (
	"net"
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

	conn, err := net.Dial("tcp", env.ProfileAddr())
	if err != nil {
		t.Fatalf("profile dial: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 4096)
	if _, err := conn.Read(buf); err != nil {
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
	if _, err := conn.Write(badLogin); err != nil {
		t.Fatalf("bad login write: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("bad login read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), `\err\256\`) {
		t.Fatalf("expected login error 256, got %q", buf[:n])
	}
}
