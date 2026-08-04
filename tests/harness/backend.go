package harness

import (
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
)

// WaitForServerCount polls FindServers until the match count equals want or timeout.
func WaitForServerCount(t *testing.T, be *backend.Backend, game, filter string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rooms, err := be.FindServers(game, filter, nil, 0)
		if err != nil {
			t.Fatalf("FindServers: %v", err)
		}
		if len(rooms) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	rooms, _ := be.FindServers(game, filter, nil, 0)
	t.Fatalf("timeout waiting for %d servers (filter %q), got %d", want, filter, len(rooms))
}
