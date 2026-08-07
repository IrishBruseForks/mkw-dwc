package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	globalLevel = levelInfo
	useColor = false
	timestamps = true
	for k := range components {
		components[k] = true
	}
	out = &bytes.Buffer{}
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	fileOut = nil
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	resetForTest()
	fn()
	buf, ok := out.(*bytes.Buffer)
	if !ok {
		t.Fatal("expected bytes.Buffer output")
	}
	return buf.String()
}

func TestLevelFilteringSuppressesDebugAtInfo(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "info",
			Color:      "never",
			Timestamps: false,
			Nas:        true,
		}); err != nil {
			t.Fatal(err)
		}
		log := For("nas")
		log.Debugf("hidden")
		log.Infof("visible")
	})

	if strings.Contains(out, "hidden") {
		t.Fatalf("debug message should be suppressed, got %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Fatalf("info message should appear, got %q", out)
	}
}

func TestDisabledStoreComponentProducesNoOutput(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "debug",
			Color:      "never",
			Timestamps: false,
			Store:      false,
		}); err != nil {
			t.Fatal(err)
		}
		For("store").Infof("should not appear")
	})

	if out != "" {
		t.Fatalf("disabled store component should produce no output, got %q", out)
	}
	if For("store") != noop {
		t.Fatal("For(store) with Store=false should return noop logger")
	}
}

func TestDisabledComponentProducesNoOutput(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "debug",
			Color:      "never",
			Timestamps: false,
			Nas:        false,
		}); err != nil {
			t.Fatal(err)
		}
		For("nas").Infof("should not appear")
	})

	if out != "" {
		t.Fatalf("disabled component should produce no output, got %q", out)
	}

	if For("nas") != noop {
		t.Fatal("For(nas) with Nas=false should return noop logger")
	}
}

func TestColorNeverHasNoANSI(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "info",
			Color:      "never",
			Timestamps: false,
			App:        true,
		}); err != nil {
			t.Fatal(err)
		}
		For("app").Warnf("plain text")
	})

	if strings.Contains(out, "\033") {
		t.Fatalf("color=never should not emit ANSI escapes, got %q", out)
	}
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "plain text") {
		t.Fatalf("expected plain warning output, got %q", out)
	}
}

func TestInitInvalidLevel(t *testing.T) {
	if err := Init(Settings{Level: "verbose"}); err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestTraceLevelVisibleAtTrace(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "trace",
			Color:      "never",
			Timestamps: false,
			Nas:        true,
		}); err != nil {
			t.Fatal(err)
		}
		For("nas").Tracef("trace line")
		For("nas").Debugf("debug line")
	})

	if !strings.Contains(out, "trace line") {
		t.Fatalf("trace message should appear, got %q", out)
	}
	if !strings.Contains(out, "debug line") {
		t.Fatalf("debug message should appear at trace level, got %q", out)
	}
}

func TestLoggerWithPrefix(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:      "info",
			Color:      "never",
			Timestamps: false,
			Qr:         true,
		}); err != nil {
			t.Fatal(err)
		}
		For("qr").With("[192.168.1.1:27900|12345678]").Infof("heartbeat")
	})

	if !strings.Contains(out, "[192.168.1.1:27900|12345678] heartbeat") {
		t.Fatalf("expected prefixed message, got %q", out)
	}
}

func TestLogDurationWarnsWhenSlow(t *testing.T) {
	out := captureOutput(t, func() {
		if err := Init(Settings{
			Level:         "warn",
			Color:         "never",
			Timestamps:    false,
			SlowThreshold: time.Millisecond,
			Backend:       true,
		}); err != nil {
			t.Fatal(err)
		}
		LogDuration("backend", "FindServers", time.Now().Add(-10*time.Millisecond))
	})

	if !strings.Contains(out, "FindServers took") {
		t.Fatalf("expected slow op warning, got %q", out)
	}
}

func TestInitInvalidColor(t *testing.T) {
	if err := Init(Settings{Color: "sometimes"}); err == nil {
		t.Fatal("expected error for invalid color")
	}
}

func TestConcurrentLogging(t *testing.T) {
	resetForTest()
	if err := Init(Settings{
		Level:      "info",
		Color:      "never",
		Timestamps: false,
		Proxy:      true,
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			For("proxy").Infof("concurrent")
		}()
	}
	wg.Wait()
}

func TestLogFileMirrorsConsole(t *testing.T) {
	resetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "app.log")

	if err := Init(Settings{
		Level:      "info",
		Color:      "never",
		Timestamps: false,
		App:        true,
		LogFile:    path,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Init(Settings{Level: "info", Color: "never"}) })

	For("app").Infof("mirrored line")

	console, ok := out.(*bytes.Buffer)
	if !ok {
		t.Fatal("expected bytes.Buffer console output")
	}
	if !strings.Contains(console.String(), "mirrored line") {
		t.Fatalf("console missing line: %q", console.String())
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "mirrored line") {
		t.Fatalf("log file missing line: %q", body)
	}
	if strings.Contains(string(body), "\033") {
		t.Fatalf("log file should be plain text, got %q", body)
	}
}

func TestLogFileTruncatesOnInit(t *testing.T) {
	resetForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("stale line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(Settings{
		Level:      "info",
		Color:      "never",
		Timestamps: false,
		App:        true,
		LogFile:    path,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Init(Settings{Level: "info", Color: "never"}) })

	For("app").Infof("fresh line")

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "stale line") {
		t.Fatalf("expected truncate on init, got %q", body)
	}
	if !strings.Contains(string(body), "fresh line") {
		t.Fatalf("expected fresh line, got %q", body)
	}
}
