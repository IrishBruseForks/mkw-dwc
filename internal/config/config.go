// Package config loads mkw-dwc.ini INI sections for the MKWii network server.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IrishBruse/mkw-dwc/internal/logging"
)

// Config holds parsed INI section key/value pairs from mkw-dwc.ini.
type Config struct {
	path     string
	sections map[string]map[string]string
}

// StoreSettings holds the required [Store] section.
type StoreSettings struct {
	Type string
	Path string
}

// Load reads an mkw-dwc.ini file.
func Load(path string) (*Config, error) {
	sections, err := parseINI(path)
	if err != nil {
		return nil, err
	}

	return &Config{
		path:     path,
		sections: sections,
	}, nil
}

// BindAddr returns the IP and Port for a named server section.
func (c *Config) BindAddr(section string) (host string, port int, err error) {
	sec, ok := c.sections[section]
	if !ok {
		return "", 0, fmt.Errorf("config: missing section %q", section)
	}

	host, ok = sec["IP"]
	if !ok || host == "" {
		return "", 0, fmt.Errorf("config: section %q missing IP", section)
	}

	portStr, ok := sec["Port"]
	if !ok || portStr == "" {
		return "", 0, fmt.Errorf("config: section %q missing Port", section)
	}

	port, err = strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		return "", 0, fmt.Errorf("config: section %q invalid Port %q: %w", section, portStr, err)
	}

	return host, port, nil
}

// NasSvcHost returns the NasServer SvcHost value.
func (c *Config) NasSvcHost() string {
	if sec, ok := c.sections["NasServer"]; ok {
		return sec["SvcHost"]
	}
	return ""
}

// Store returns required [Store] Type and Path settings.
func (c *Config) Store() (StoreSettings, error) {
	sec, ok := c.sections["Store"]
	if !ok {
		return StoreSettings{}, fmt.Errorf("config: missing section %q", "Store")
	}

	kind, ok := sec["Type"]
	if !ok || kind == "" {
		return StoreSettings{}, fmt.Errorf(`config: section "Store" missing Type ("json")`)
	}
	kind = strings.ToLower(kind)
	switch kind {
	case "json":
	default:
		return StoreSettings{}, fmt.Errorf(`config: section "Store" invalid Type %q (want "json")`, kind)
	}

	path, ok := sec["Path"]
	if !ok || path == "" {
		return StoreSettings{}, fmt.Errorf(`config: section "Store" missing Path`)
	}

	return StoreSettings{Type: kind, Path: path}, nil
}

// LoggingSettings returns parsed [Logging] section values.
func (c *Config) LoggingSettings() (logging.Settings, error) {
	s := logging.Settings{
		Level:         "info",
		Color:         "auto",
		Timestamps:    true,
		SlowThreshold: time.Second,
		Rotate:        "off",
		Nas:           true,
		Profile:       true,
		Gpsp:          true,
		Qr:            true,
		Browser:       true,
		Natneg:        true,
		Proxy:         true,
		App:           true,
		Store:         true,
		Backend:       true,
	}

	sec, ok := c.sections["Logging"]
	if !ok {
		return s, nil
	}

	if v, ok := sec["Level"]; ok && v != "" {
		s.Level = v
	}
	if err := validateLogLevel(s.Level); err != nil {
		return logging.Settings{}, err
	}

	if v, ok := sec["Color"]; ok && v != "" {
		s.Color = strings.ToLower(strings.TrimSpace(v))
	}
	if err := validateLogColor(s.Color); err != nil {
		return logging.Settings{}, err
	}

	var err error
	if v, ok := sec["Timestamps"]; ok && v != "" {
		s.Timestamps, err = parseBool(v)
		if err != nil {
			return logging.Settings{}, fmt.Errorf(`config: section "Logging" invalid Timestamps %q: %w`, v, err)
		}
	}

	if v, ok := sec["SlowThreshold"]; ok && v != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return logging.Settings{}, fmt.Errorf(`config: section "Logging" invalid SlowThreshold %q: %w`, v, err)
		}
		s.SlowThreshold = d
	}
	if v, ok := sec["Rotate"]; ok && v != "" {
		s.Rotate = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := sec["LogFile"]; ok {
		s.LogFile = strings.TrimSpace(v)
	}

	compSec := c.sections["LoggingComponents"]
	if s.Nas, err = parseLoggingBool(compSec, "LoggingComponents", "Nas", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Profile, err = parseLoggingBool(compSec, "LoggingComponents", "Profile", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Gpsp, err = parseLoggingBool(compSec, "LoggingComponents", "Gpsp", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Qr, err = parseLoggingBool(compSec, "LoggingComponents", "Qr", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Browser, err = parseLoggingBool(compSec, "LoggingComponents", "Browser", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Natneg, err = parseLoggingBool(compSec, "LoggingComponents", "Natneg", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Proxy, err = parseLoggingBool(compSec, "LoggingComponents", "Proxy", true); err != nil {
		return logging.Settings{}, err
	}
	if s.App, err = parseLoggingBool(compSec, "LoggingComponents", "App", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Store, err = parseLoggingBool(compSec, "LoggingComponents", "Store", true); err != nil {
		return logging.Settings{}, err
	}
	if s.Backend, err = parseLoggingBool(compSec, "LoggingComponents", "Backend", true); err != nil {
		return logging.Settings{}, err
	}

	return s, nil
}

// HTTPDumpFile returns optional [Logging] DumpFile for raw NAS/proxy TCP dumps.
// Owned by httpfix (via app wiring), not logging.Init.
func (c *Config) HTTPDumpFile() string {
	if sec, ok := c.sections["Logging"]; ok {
		return strings.TrimSpace(sec["DumpFile"])
	}
	return ""
}

// SectionBool returns a boolean from a section key, or defaultVal if unset.
func (c *Config) SectionBool(section, key string, defaultVal bool) (bool, error) {
	sec, ok := c.sections[section]
	if !ok {
		return defaultVal, nil
	}
	v, ok := sec[key]
	if !ok || v == "" {
		return defaultVal, nil
	}
	b, err := parseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: section %q invalid %s %q: %w", section, key, v, err)
	}
	return b, nil
}

func validateLogLevel(level string) error {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace", "debug", "info", "warn", "error", "":
		return nil
	default:
		return fmt.Errorf(`config: section "Logging" invalid Level %q (want trace, debug, info, warn, or error)`, level)
	}
}

func validateLogColor(color string) error {
	switch strings.ToLower(strings.TrimSpace(color)) {
	case "auto", "always", "never", "":
		return nil
	default:
		return fmt.Errorf(`config: section "Logging" invalid Color %q (want auto, always, or never)`, color)
	}
}

func parseLoggingBool(sec map[string]string, section, key string, defaultVal bool) (bool, error) {
	if sec == nil {
		return defaultVal, nil
	}
	v, ok := sec[key]
	if !ok || v == "" {
		return defaultVal, nil
	}
	b, err := parseBool(v)
	if err != nil {
		return false, fmt.Errorf(`config: section %q invalid %s %q: %w`, section, key, v, err)
	}
	return b, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("want true, false, 1, or 0")
	}
}

func parseINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	sections := make(map[string]map[string]string)
	var current string

	scanner := bufio.NewScanner(f)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			if end <= 1 {
				return nil, fmt.Errorf("config: %q:%d: invalid section header", path, lineNo)
			}
			current = line[1:end]
			if sections[current] == nil {
				sections[current] = make(map[string]string)
			}
			continue
		}

		if current == "" {
			return nil, fmt.Errorf("config: %q:%d: key outside section", path, lineNo)
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			sections[current][strings.TrimSpace(key)] = ""
			continue
		}

		sections[current][strings.TrimSpace(key)] = unquote(stripInlineComment(strings.TrimSpace(value)))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	return sections, nil
}

func stripInlineComment(s string) string {
	inQuote := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quote {
				inQuote = false
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = true
			quote = c
			continue
		}
		if c == '#' || c == ';' {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
