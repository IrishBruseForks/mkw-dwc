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

	prof, err := s.GetProfile(profileid)
	if err != nil || prof == nil {
		t.Fatalf("GetProfile: %v %#v", err, prof)
	}
	if prof.UserID != userid || prof.Uniquenick == "" || prof.PID != "11" {
		t.Fatalf("unexpected profile: %+v", prof)
	}

	if err := s.UpdateProfile(profileid, map[string]string{
		"firstname": "Wii:test@ABCD",
		"lastname":  "Player",
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	prof2, err := s.GetProfile(profileid)
	if err != nil {
		t.Fatalf("GetProfile after update: %v", err)
	}
	if prof2.Firstname != "Wii:test@ABCD" || prof2.Lastname != "Player" {
		t.Fatalf("updated profile: %+v", prof2)
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

func TestStoreNasLoginAuthtokenCollision(t *testing.T) {
	dir := t.TempDir()
	s, err := jsonstore.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if err := s.Initialize(); err != nil {
		t.Fatalf("init: %v", err)
	}

	userid1 := "0000000000002"
	userid2 := "0000000000003"
	auth1 := map[string]string{"userid": userid1, "gsbrcd": "RMCJ0"}
	auth2 := map[string]string{"userid": userid2, "gsbrcd": "RMCJ0"}

	if err := s.StoreNasLogin(userid1, "shared-token", auth1); err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := s.StoreNasLogin(userid2, "shared-token", auth2); err == nil {
		t.Fatal("expected authtoken collision error")
	} else if !strings.Contains(err.Error(), "authtoken collision") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreNasLoginUpdateToken(t *testing.T) {
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
	auth := map[string]string{"userid": userid, "gsbrcd": "RMCJ0", "ingamesn": "Mii"}

	if err := s.StoreNasLogin(userid, "token-old", auth); err != nil {
		t.Fatalf("first store: %v", err)
	}
	if err := s.StoreNasLogin(userid, "token-new", auth); err != nil {
		t.Fatalf("update token: %v", err)
	}

	got, err := s.GetNasLogin("token-new")
	if err != nil || got == nil {
		t.Fatalf("get new token: %v %#v", err, got)
	}
	if got["userid"] != userid {
		t.Fatalf("userid = %q", got["userid"])
	}

	old, err := s.GetNasLogin("token-old")
	if err != nil {
		t.Fatalf("get old token: %v", err)
	}
	if old != nil {
		t.Fatalf("old token should be gone, got %#v", old)
	}
}

func TestStoreNasLoginIdempotentRestore(t *testing.T) {
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
	auth := map[string]string{"userid": userid, "gsbrcd": "RMCJ0", "devname": "Wii"}

	if err := s.StoreNasLogin(userid, "token-same", auth); err != nil {
		t.Fatalf("first store: %v", err)
	}
	auth["devname"] = "Wii2"
	if err := s.StoreNasLogin(userid, "token-same", auth); err != nil {
		t.Fatalf("re-store same token: %v", err)
	}

	got, err := s.GetNasLogin("token-same")
	if err != nil || got == nil {
		t.Fatalf("get token: %v %#v", err, got)
	}
	if got["devname"] == auth["devname"] {
		t.Fatalf("devname should be base64-encoded, got %q", got["devname"])
	}
}
