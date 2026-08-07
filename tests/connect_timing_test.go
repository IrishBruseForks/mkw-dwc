package tests_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

// TestConnectPathTiming measures NAS -> profile -> GPSP wall time for the WFC path.
func TestConnectPathTiming(t *testing.T) {
	env := harness.Start(t, openJSONStore(t))

	t0 := time.Now()
	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCE01")
	tAcct := time.Since(t0)

	t1 := time.Now()
	token, challenge := harness.NASLogin(t, env.NASURL(), userid, "RMCE01", "RMCE01", "TestMii")
	tNas := time.Since(t1)

	t2 := time.Now()
	pid := harness.ProfileLogin(t, env.ProfileAddr(), token, challenge)
	tProf := time.Since(t2)

	t3 := time.Now()
	reply := harness.GPSPOthersList(t, env.GPSPAddr(), strconv.FormatInt(pid, 10), "1", "0")
	tGpsp := time.Since(t3)

	total := time.Since(t0)
	t.Logf("acctcreate=%s nas_login=%s profile_login=%s gpsp=%s total=%s reply_len=%d",
		tAcct, tNas, tProf, tGpsp, total, len(reply))
	fmt.Printf("TIMING acctcreate=%s nas_login=%s profile_login=%s gpsp=%s total=%s\n",
		tAcct, tNas, tProf, tGpsp, total)

	// Local loopback WFC path should finish well under a second.
	if total > time.Second {
		t.Fatalf("WFC path too slow: %s", total)
	}
}
