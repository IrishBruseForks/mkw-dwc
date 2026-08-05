package tests_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/database"
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

	conn, profileID, sesskey, uniquenick := harness.ProfileSession(t, env.ProfileAddr(), token, acChallenge)
	conn.Close()

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
