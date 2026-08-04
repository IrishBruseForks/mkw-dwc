package json_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonstore "github.com/IrishBruse/mkw-dwc/internal/database/json"
)

func TestJSONStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := jsonstore.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.Initialize(); err != nil {
		t.Fatalf("init: %v", err)
	}

	userid := s.GetNextAvailableUserid()
	if userid != "0000000000002" {
		t.Fatalf("first userid = %q", userid)
	}

	auth := map[string]string{
		"userid": userid,
		"gsbrcd": "RMCJ0",
		"csnum":  "LU123456789",
		"ingamesn": "TestMii",
	}
	if err := s.StoreNasLogin(userid, "token-1", auth); err != nil {
		t.Fatalf("store nas: %v", err)
	}

	got, err := s.GetNasLogin("token-1")
	if err != nil || got == nil {
		t.Fatalf("get nas: %v %#v", err, got)
	}
	if got["userid"] != userid {
		t.Fatalf("userid = %q", got["userid"])
	}

	uid, profileid, gsbrcd, uniquenick, err := s.LoginProfileFromAuth(auth)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if uid != userid || profileid != 1 || gsbrcd != "RMCJ0" || uniquenick == "" {
		t.Fatalf("login result uid=%s pid=%d gsbrcd=%s nick=%s", uid, profileid, gsbrcd, uniquenick)
	}

	sesskey, ticket, err := s.CreateSession(profileid)
	if err != nil || sesskey == "" || ticket == "" {
		t.Fatalf("session: %v %q %q", err, sesskey, ticket)
	}
	if strings.ContainsAny(ticket, "+/=") {
		t.Fatalf("login ticket should use GameSpy base64, got %q", ticket)
	}

	sn, err := s.GetIngameSN(profileid)
	if err != nil {
		t.Fatalf("GetIngameSN: %v", err)
	}
	if sn == "" {
		t.Fatal("expected ingamesn from NAS login")
	}

	if err := s.DeleteSession(sesskey); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	// Reload from disk
	s2, err := jsonstore.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got2, err := s2.GetNasLogin("token-1")
	if err != nil || got2 == nil {
		t.Fatalf("reload nas: %v %#v", err, got2)
	}
	for _, name := range []string{"users.json", "sessions.json", "nas_logins.json", "banned.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}
