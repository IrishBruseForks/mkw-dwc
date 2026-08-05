package httpfix_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/httpfix"
	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

func TestWrapListenerDedupeHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "host=%s path=%s", r.Host, r.URL.Path)
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go srv.Serve(httpfix.WrapListener(ln, "nas"))
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req := "" +
		"GET /ac HTTP/1.1\r\n" +
		"Host: naswii.nintendowifi.net\r\n" +
		"Host: naswii.nintendowifi.net\r\n" +
		"Connection: close\r\n" +
		"\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	if got := string(body); !strings.Contains(got, "host=naswii.nintendowifi.net") {
		t.Fatalf("body %q", got)
	}
}

func TestWrapListenerSingleHostUnchanged(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go srv.Serve(httpfix.WrapListener(ln, "nas"))
	defer srv.Close()

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "ok" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
}

func TestDumpFileLogsTraffic(t *testing.T) {
	var dumpBuf bytes.Buffer
	httpfix.SetDumpOutput(&dumpBuf)
	t.Cleanup(func() { httpfix.SetDumpOutput(nil) })

	if err := logging.Init(logging.Settings{
		Level:      "info",
		Color:      "never",
		Timestamps: false,
		Nas:        true,
	}); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go srv.Serve(httpfix.WrapListener(ln, "nas"))
	defer srv.Close()

	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	out := dumpBuf.String()
	if !strings.Contains(out, "accept ") {
		t.Fatalf("expected accept dump, got %q", out)
	}
	if !strings.Contains(out, "recv ") {
		t.Fatalf("expected recv dump, got %q", out)
	}
	if !strings.Contains(out, "send ") {
		t.Fatalf("expected send dump, got %q", out)
	}
	if !strings.Contains(out, "text:\n") || !strings.Contains(out, "hex:\n") {
		t.Fatalf("expected verbose text and hex sections, got %q", out)
	}
}

func TestSetDumpFileCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "missing", "traffic.log")
	if err := httpfix.SetDumpFile(path); err != nil {
		t.Fatalf("SetDumpFile: %v", err)
	}
	t.Cleanup(func() { _ = httpfix.SetDumpFile("") })

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dump file missing: %v", err)
	}
}

func TestWrapListenerKeepAliveDedupeHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var hosts []string
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hosts = append(hosts, r.Host)
			_, _ = io.WriteString(w, "ok")
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go srv.Serve(httpfix.WrapListener(ln, "nas"))
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	br := bufio.NewReader(conn)

	writeDupHost := func(path string) {
		t.Helper()
		req := "" +
			"POST " + path + " HTTP/1.1\r\n" +
			"Host: naswii.nintendowifi.net\r\n" +
			"Host: naswii.nintendowifi.net\r\n" +
			"Content-Type: application/x-www-form-urlencoded\r\n" +
			"Content-Length: 11\r\n" +
			"\r\n" +
			"action=test"
		if _, err := io.WriteString(conn, req); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatalf("ReadResponse: %v", err)
		}
		defer resp.Body.Close()
		_, _ = io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}

	writeDupHost("/one")
	writeDupHost("/two")

	if len(hosts) != 2 {
		t.Fatalf("got %d requests, want 2 (%v)", len(hosts), hosts)
	}
	for i, h := range hosts {
		if h != "naswii.nintendowifi.net" {
			t.Fatalf("request %d host %q", i, h)
		}
	}
}

func TestTLSHandshakeWarned(t *testing.T) {
	var logBuf bytes.Buffer
	var dumpBuf bytes.Buffer
	httpfix.SetDumpOutput(&dumpBuf)
	t.Cleanup(func() { httpfix.SetDumpOutput(nil) })

	if err := logging.Init(logging.Settings{
		Level:      "info",
		Color:      "never",
		Timestamps: false,
		Proxy:      true,
	}); err != nil {
		t.Fatal(err)
	}
	logging.SetOutputForTest(&logBuf)
	t.Cleanup(func() { logging.SetOutputForTest(nil) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: 500 * time.Millisecond,
	}
	go srv.Serve(httpfix.WrapListener(ln, "proxy"))
	defer srv.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// Minimal TLS ClientHello record header + payload.
	tlsHello := []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00}
	if _, err := conn.Write(tlsHello); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	time.Sleep(50 * time.Millisecond)

	logOut := logBuf.String()
	if !strings.Contains(logOut, "TLS handshake") {
		t.Fatalf("expected TLS warning in logs, got %q", logOut)
	}
	if !strings.Contains(logOut, "NoSSL") {
		t.Fatalf("expected NoSSL hint in logs, got %q", logOut)
	}
	if strings.Contains(logOut, "hex:") {
		t.Fatalf("raw TLS dump should not appear in stdout logs, got %q", logOut)
	}

	dumpOut := dumpBuf.String()
	if !strings.Contains(dumpOut, "tls ") {
		t.Fatalf("expected TLS dump in dump file, got %q", dumpOut)
	}
	if !strings.Contains(dumpOut, "hex:") {
		t.Fatalf("expected hex dump in dump file, got %q", dumpOut)
	}
}
