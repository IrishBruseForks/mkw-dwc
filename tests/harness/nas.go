package harness

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

// NASForm encodes fields the way the NAS server expects.
func NASForm(fields map[string]string) string {
	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		enc := base64.StdEncoding.EncodeToString([]byte(v))
		enc = strings.ReplaceAll(enc, "=", "*")
		parts = append(parts, k+"="+enc)
	}
	return strings.Join(parts, "&") + "\r\n"
}

// NASPost posts a form body to url and returns the decoded response body as key/value pairs.
func NASPost(t *testing.T, url, body string) map[string]string {
	t.Helper()
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatalf("nas post %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("nas read: %v", err)
	}
	return NASParseBody(t, string(raw))
}

// NASParseBody decodes a NAS response body into plain string values.
func NASParseBody(t *testing.T, body string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for _, pair := range strings.Split(strings.TrimSpace(body), "&") {
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		v = strings.ReplaceAll(v, "*", "=")
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			t.Fatalf("decode %s: %v", k, err)
		}
		out[k] = string(raw)
	}
	return out
}

// NASAcctCreate creates an account and returns the new userid.
func NASAcctCreate(t *testing.T, acURL, gamecd string) string {
	t.Helper()
	ret := NASPost(t, acURL, NASForm(map[string]string{
		"action": "acctcreate",
		"gamecd": gamecd,
	}))
	if ret["returncd"] != "002" {
		t.Fatalf("acctcreate returncd=%q body=%v", ret["returncd"], ret)
	}
	if ret["userid"] == "" {
		t.Fatalf("acctcreate missing userid: %v", ret)
	}
	return ret["userid"]
}

// NASLogin logs in and returns userid, authtoken, and acChallenge.
func NASLogin(t *testing.T, acURL, userid, gsbrcd, gamecd, ingamesn string) (authtoken, acChallenge string) {
	t.Helper()
	fields := map[string]string{
		"action": "login",
		"userid": userid,
		"gsbrcd": gsbrcd,
		"gamecd": gamecd,
	}
	if ingamesn != "" {
		fields["ingamesn"] = ingamesn
	}
	ret := NASPost(t, acURL, NASForm(fields))
	if ret["returncd"] != "001" {
		t.Fatalf("login returncd=%q body=%v", ret["returncd"], ret)
	}
	if ret["token"] == "" || ret["challenge"] == "" {
		t.Fatalf("login missing token/challenge: %v", ret)
	}
	if ret["locator"] != "gamespy.com" {
		t.Fatalf("login locator=%q", ret["locator"])
	}
	return ret["token"], ret["challenge"]
}

// NASSvcLoc calls svcloc and returns response fields.
func NASSvcLoc(t *testing.T, acURL, userid, svc string) map[string]string {
	t.Helper()
	ret := NASPost(t, acURL, NASForm(map[string]string{
		"action": "svcloc",
		"userid": userid,
		"svc":    svc,
	}))
	if ret["returncd"] != "007" {
		t.Fatalf("svcloc returncd=%q body=%v", ret["returncd"], ret)
	}
	if ret["statusdata"] != "Y" {
		t.Fatalf("svcloc statusdata=%q", ret["statusdata"])
	}
	return ret
}
