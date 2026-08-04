package harness

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
)

// ProfileLogin performs a full GPCM login and returns profileid.
func ProfileLogin(t *testing.T, addr, authtoken, acChallenge string) int64 {
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
	serverChallenge := GameSpyField(string(buf[:n]), "challenge")
	if serverChallenge == "" {
		t.Fatalf("missing server challenge: %q", buf[:n])
	}

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
	profileIDStr := GameSpyField(msg, "profileid")
	profileID, _ := strconv.ParseInt(profileIDStr, 10, 64)
	if profileID == 0 {
		t.Fatalf("profile login missing profileid: %q", msg)
	}
	if GameSpyField(msg, "sesskey") == "" {
		t.Fatalf("profile login missing sesskey: %q", msg)
	}
	if GameSpyField(msg, "proof") == "" {
		t.Fatalf("profile login missing proof: %q", msg)
	}
	return profileID
}

// ProfileKA sends keep-alive and expects a reply.
func ProfileKA(t *testing.T, addr string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("profile dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	buf := make([]byte, 4096)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("profile lc read: %v", err)
	}

	ka := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "ka",
		"__cmd_val__": "",
	})
	if _, err := conn.Write(ka); err != nil {
		t.Fatalf("profile ka write: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("profile ka read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), `\ka\`) {
		t.Fatalf("expected ka reply, got %q", buf[:n])
	}
}
