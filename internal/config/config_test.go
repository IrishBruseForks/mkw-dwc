package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/config"
)

func TestStoreRequired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-store.ini")
	if err := os.WriteFile(path, []byte("[NasServer]\nIP = 0.0.0.0\nPort = 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := cfg.Store(); err == nil {
		t.Fatal("expected missing Store section error")
	}
}

func TestStoreTypeMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-type.ini")
	content := "[Store]\nPath = \"data\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, err = cfg.Store()
	if err == nil || !strings.Contains(err.Error(), "missing Type") {
		t.Fatalf("expected missing Type error, got %v", err)
	}
}

func TestStoreParsesQuotedInlineComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.ini")
	content := `[Store]
Type = "json" # "json"
Path = "data" # JSON data directory
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	st, err := cfg.Store()
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if st.Type != "json" || st.Path != "data" {
		t.Fatalf("got %+v", st)
	}
}

func TestStoreInvalidType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-type.ini")
	content := `[Store]
Type = "redis"
Path = "data"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, err = cfg.Store()
	if err == nil || !strings.Contains(err.Error(), "invalid Type") {
		t.Fatalf("expected invalid Type error, got %v", err)
	}
}

func TestLoggingDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-logging.ini")
	content := `[Store]
Type = "json"
Path = "data"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s, err := cfg.LoggingSettings()
	if err != nil {
		t.Fatalf("logging: %v", err)
	}
	if s.Level != "info" || s.Color != "auto" || !s.Timestamps {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	if !s.Nas || !s.Profile || !s.Gpsp || !s.Qr || !s.Browser || !s.Natneg || !s.Proxy || !s.App {
		t.Fatalf("service defaults should be true: %+v", s)
	}
	if s.DumpFile != "" {
		t.Fatalf("DumpFile default should be empty: %+v", s)
	}
	if s.LogFile != "" {
		t.Fatalf("LogFile default should be empty: %+v", s)
	}
}

func TestLoggingInvalidLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-level.ini")
	content := `[Store]
Type = "json"
Path = "data"

[Logging]
Level = verbose
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, err = cfg.LoggingSettings()
	if err == nil || !strings.Contains(err.Error(), "invalid Level") {
		t.Fatalf("expected invalid Level error, got %v", err)
	}
}

func TestLoggingParsesSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logging.ini")
	content := `[Store]
Type = "json"
Path = "data"

[Logging]
Level = DEBUG
Color = never
Timestamps = false
Nas = 0
Profile = 1
Qr = false
Browser = true
Natneg = false
Proxy = 1
App = 0
LogFile = logs/app.log
DumpFile = logs/traffic.log
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	s, err := cfg.LoggingSettings()
	if err != nil {
		t.Fatalf("logging: %v", err)
	}
	if s.Level != "DEBUG" || s.Color != "never" || s.Timestamps {
		t.Fatalf("unexpected level/color/timestamps: %+v", s)
	}
	want := struct {
		Nas, Profile, Gpsp, Qr, Browser, Natneg, Proxy, App bool
		LogFile, DumpFile                                   string
	}{false, true, true, false, true, false, true, false, "logs/app.log", "logs/traffic.log"}
	if s.Nas != want.Nas || s.Profile != want.Profile || s.Gpsp != want.Gpsp || s.Qr != want.Qr ||
		s.Browser != want.Browser || s.Natneg != want.Natneg || s.Proxy != want.Proxy ||
		s.App != want.App || s.LogFile != want.LogFile || s.DumpFile != want.DumpFile {
		t.Fatalf("unexpected service flags: got %+v want %+v", s, want)
	}
}
