package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
	if err := Init(Settings{Level: "trace"}); err == nil {
		t.Fatal("expected error for invalid level")
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
