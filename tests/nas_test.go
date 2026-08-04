package tests_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestNASReferenceHandlers(t *testing.T) {
	gpcm := openJSONStore(t)
	env := harness.Start(t, gpcm)

	harness.NASHealth(t, env.NASRootURL())

	userid := harness.NASAcctCreate(t, env.NASURL(), "RMCJ01")
	token, challenge := harness.NASLogin(t, env.NASURL(), userid, "RMCJ01", "RMCJ01", "TestMii")
	if token == "" || challenge == "" {
		t.Fatal("expected login token and challenge")
	}

	svc9000 := harness.NASSvcLoc(t, env.NASURL(), userid, "9000")
	if svc9000["svchost"] != "dls1.nintendowifi.net" {
		t.Fatalf("svc9000 svchost=%q", svc9000["svchost"])
	}
	if svc9000["token"] == "" {
		t.Fatal("svc9000 missing token")
	}

	svc9001 := harness.NASSvcLoc(t, env.NASURL(), userid, "9001")
	if svc9001["servicetoken"] == "" {
		t.Fatal("svc9001 missing servicetoken")
	}

	svc0000 := harness.NASSvcLoc(t, env.NASURL(), userid, "0000")
	if svc0000["svchost"] != "n/a" {
		t.Fatalf("svc0000 svchost=%q", svc0000["svchost"])
	}
	if svc0000["servicetoken"] == "" {
		t.Fatal("svc0000 missing servicetoken")
	}

	prBody := harness.NASForm(map[string]string{
		"words": "bad\tword",
	})
	resp, err := http.Post(
		strings.Replace(env.NASURL(), "/ac", "/pr", 1),
		"application/x-www-form-urlencoded",
		strings.NewReader(prBody),
	)
	if err != nil {
		t.Fatalf("pr post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	ret := harness.NASParseBody(t, string(raw))
	if ret["returncd"] != "000" {
		t.Fatalf("pr returncd=%q", ret["returncd"])
	}
	if ret["prwords"] != "00" {
		t.Fatalf("prwords=%q want 00", ret["prwords"])
	}
}
