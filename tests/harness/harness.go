// Package harness starts mkw-dwc services for integration tests.
package harness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/backend"
	"github.com/IrishBruse/mkw-dwc/internal/config"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/browser"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/gpsp"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/natneg"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/profile"
	"github.com/IrishBruse/mkw-dwc/internal/gamespy/qr"
	"github.com/IrishBruse/mkw-dwc/internal/nas"
	"github.com/IrishBruse/mkw-dwc/internal/database"
)

// Env holds addresses for a running test stack.
type Env struct {
	Backend *backend.Backend
	Store   database.Store
	Keys    map[string]string

	NASPort     int
	ProfilePort int
	QRPort      int
	BrowserPort int
	NatNegPort  int
	GPSPPort    int

	cancel context.CancelFunc
}

// Start boots NAS, profile, QR, browser, NATNEG, and GPSP on ephemeral ports.
func Start(t *testing.T, gpcm database.Store) *Env {
	t.Helper()

	keys := gamespy.SecretKeys()
	be := backend.New()

	env := &Env{
		Backend:     be,
		Store:       gpcm,
		Keys:        keys,
		NASPort:     FreeTCPPort(t),
		ProfilePort: FreeTCPPort(t),
		QRPort:      FreeUDPPort(t),
		BrowserPort: FreeTCPPort(t),
		NatNegPort:  FreeUDPPort(t),
		GPSPPort:    FreeTCPPort(t),
	}

	qrSrv := qr.New(fmt.Sprintf(":%d", env.QRPort), be, keys)
	qrSrv.Profiles = gpcm
	browserSrv := browser.New(fmt.Sprintf(":%d", env.BrowserPort), be, keys, qrSrv)
	nasSrv := &nas.Server{
		DB:      gpcm,
		SvcHost: "dls1.nintendowifi.net",
		Addr:    fmt.Sprintf(":%d", env.NASPort),
	}
	profileSrv := &profile.Server{DB: gpcm, Addr: fmt.Sprintf(":%d", env.ProfilePort)}
	gpspSrv := &gpsp.Server{DB: gpcm, Addr: fmt.Sprintf(":%d", env.GPSPPort)}
	natnegSrv := natneg.New(fmt.Sprintf(":%d", env.NatNegPort), be)

	ctx, cancel := context.WithCancel(context.Background())
	env.cancel = cancel
	t.Cleanup(func() {
		cancel()
		// Let listeners finish closing before TempDir cleanup.
		time.Sleep(100 * time.Millisecond)
	})

	go nasSrv.Serve(ctx)
	go profileSrv.Serve(ctx)
	go gpspSrv.Serve(ctx)
	go qrSrv.Serve(ctx)
	go browserSrv.Serve(ctx)
	go natnegSrv.Serve(ctx)

	WaitTCP(env.NASPort, 3*time.Second)
	WaitTCP(env.ProfilePort, 3*time.Second)
	WaitTCP(env.GPSPPort, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	return env
}

// NASURL returns the NAS /ac endpoint.
func (e *Env) NASURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/ac", e.NASPort)
}

// NASRootURL returns the NAS health-check endpoint.
func (e *Env) NASRootURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", e.NASPort)
}

// ProfileAddr returns the profile TCP dial address.
func (e *Env) ProfileAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", e.ProfilePort)
}

// QRAddr returns the QR UDP dial address.
func (e *Env) QRAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", e.QRPort)
}

// BrowserAddr returns the browser TCP dial address.
func (e *Env) BrowserAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", e.BrowserPort)
}

// NatNegAddr returns the NATNEG UDP dial address.
func (e *Env) NatNegAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", e.NatNegPort)
}

// GPSPAddr returns the GPSP TCP dial address.
func (e *Env) GPSPAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", e.GPSPPort)
}

// LoadConfig loads mkw-dwc.ini from the module root.
func LoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(ModuleRoot(), "mkw-dwc.ini"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg
}

// ModuleRoot finds the directory containing go.mod.
func ModuleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// FreeTCPPort returns an ephemeral TCP port.
func FreeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free tcp: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// FreeUDPPort returns an ephemeral UDP port.
func FreeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free udp: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

// WaitTCP blocks until something accepts on port or timeout elapses.
func WaitTCP(port int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// NASHealth checks GET / returns ok.
func NASHealth(t *testing.T, rootURL string) {
	t.Helper()
	resp, err := http.Get(rootURL)
	if err != nil {
		t.Fatalf("nas get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nas status = %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Organization") != "Nintendo" {
		t.Fatalf("missing X-Organization header")
	}
}
