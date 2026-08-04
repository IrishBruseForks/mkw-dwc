package tests_test

import (
	"net"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/database"
	dbjson "github.com/IrishBruse/mkw-dwc/internal/database/json"
	"github.com/IrishBruse/mkw-dwc/tests/harness"
)

func TestHealthChecksJSON(t *testing.T) {
	runHealthChecks(t, openJSONStore(t))
}

func runHealthChecks(t *testing.T, gpcm database.Store) {
	t.Helper()
	_ = harness.LoadConfig(t)
	if gamespy.SecretKeys()["mariokartwii"] != "9r3Rmy" {
		t.Fatalf("unexpected mariokartwii secret")
	}

	env := harness.Start(t, gpcm)
	harness.NASHealth(t, env.NASRootURL())

	conn, err := net.Dial("tcp", env.ProfileAddr())
	if err != nil {
		t.Fatalf("profile dial: %v", err)
	}
	defer conn.Close()
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("profile read: %v", err)
	}
	msg := string(buf[:n])
	if !strings.Contains(msg, `\lc\1`) || !strings.Contains(msg, `\challenge\`) {
		t.Fatalf("expected lc challenge, got %q", msg)
	}
}

func openJSONStore(t *testing.T) *dbjson.Store {
	t.Helper()
	s, err := dbjson.Open(t.TempDir())
	if err != nil {
		t.Fatalf("jsonstore open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Initialize(); err != nil {
		t.Fatalf("jsonstore init: %v", err)
	}
	return s
}
