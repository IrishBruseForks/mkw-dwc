package harness

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
)

// GPSPOthersList dials GPSP, sends an otherslist request, and returns the reply.
func GPSPOthersList(t *testing.T, addr, profileID, sesskey, opids string) string {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("gpsp dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	numopids := "0"
	if opids != "" {
		numopids = strconv.Itoa(len(strings.Split(opids, "|")))
	}

	msg := gamespy.CreateGameSpyMessage(map[string]string{
		"__cmd__":     "otherslist",
		"__cmd_val__": "",
		"sesskey":     sesskey,
		"profileid":   profileID,
		"numopids":    numopids,
		"opids":       opids,
		"namespaceid": "16",
		"gamename":    "mariokartwii",
	})
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("gpsp otherslist write: %v", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("gpsp otherslist read: %v", err)
	}
	return string(buf[:n])
}
