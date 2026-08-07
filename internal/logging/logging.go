package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Settings configures global logging behavior.
type Settings struct {
	Level         string        // trace, debug, info, warn, error (default info)
	Color         string        // auto, always, never (default auto)
	Timestamps    bool          // default true
	SlowThreshold time.Duration // warn when ops exceed this (0 = off)
	Rotate        string        // off, daily (default off)
	Nas           bool          // default true
	Profile       bool
	Gpsp          bool
	Qr            bool
	Browser       bool
	Natneg        bool
	Proxy         bool
	App           bool
	Store         bool
	Backend       bool
	LogFile string // optional mirror of user-facing logs (empty = stderr only)
}

type level int

const (
	levelTrace level = iota
	levelDebug
	levelInfo
	levelWarn
	levelError
)

// Logger writes leveled messages for a single component.
type Logger struct {
	component string
	prefix    string
	enabled   bool
}

var (
	mu            sync.Mutex
	globalLevel   = levelInfo
	slowThreshold time.Duration
	useColor      bool
	timestamps    = true
	components    = map[string]bool{
		"nas":     true,
		"profile": true,
		"gpsp":    true,
		"qr":      true,
		"browser": true,
		"natneg":  true,
		"proxy":   true,
		"app":     true,
		"store":   true,
		"backend": true,
	}
	out     io.Writer = os.Stderr
	fileOut io.Writer
	logFile *os.File

	noop = &Logger{enabled: false}

	levelTag = map[level]string{
		levelTrace: "TRACE",
		levelDebug: "DEBUG",
		levelInfo:  "INFO",
		levelWarn:  "WARN",
		levelError: "ERROR",
	}

	levelColor = map[level]string{
		levelTrace: "\033[2m",
		levelDebug: "\033[2m",
		levelInfo:  "\033[36m",
		levelWarn:  "\033[33m",
		levelError: "\033[31m",
	}

	componentColor = "\033[35m"
	colorReset     = "\033[0m"
)

// Init parses level and color settings and configures global logging state.
func Init(s Settings) error {
	lvl, err := parseLevel(s.Level)
	if err != nil {
		return err
	}

	color := strings.ToLower(strings.TrimSpace(s.Color))
	if color == "" {
		color = "auto"
	}
	var colorOn bool
	switch color {
	case "auto":
		colorOn = stderrIsTerminal()
	case "always":
		colorOn = true
	case "never":
		colorOn = false
	default:
		return fmt.Errorf("logging: invalid color %q (want auto, always, or never)", s.Color)
	}

	var newFile *os.File
	if path := strings.TrimSpace(s.LogFile); path != "" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("logging: create log dir %q: %w", dir, err)
			}
		}
		if err := rotateLogFile(path, s.Rotate); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("logging: open log file %q: %w", path, err)
		}
		newFile = f
	}

	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	fileOut = nil
	if newFile != nil {
		logFile = newFile
		fileOut = newFile
	}

	globalLevel = lvl
	slowThreshold = s.SlowThreshold
	useColor = colorOn
	timestamps = s.Timestamps
	components["nas"] = s.Nas
	components["profile"] = s.Profile
	components["gpsp"] = s.Gpsp
	components["qr"] = s.Qr
	components["browser"] = s.Browser
	components["natneg"] = s.Natneg
	components["proxy"] = s.Proxy
	components["app"] = s.App
	components["store"] = s.Store
	components["backend"] = s.Backend

	return nil
}

func rotateLogFile(path, rotate string) error {
	switch strings.ToLower(strings.TrimSpace(rotate)) {
	case "", "off":
		return nil
	case "daily":
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("logging: stat log file %q: %w", path, err)
		}
		if info.Size() == 0 {
			return nil
		}
		backup := path + "." + time.Now().Format("20060102")
		if err := os.Rename(path, backup); err != nil {
			return fmt.Errorf("logging: rotate log file %q: %w", path, err)
		}
		return nil
	default:
		return fmt.Errorf("logging: invalid Rotate %q (want off or daily)", rotate)
	}
}

// SetOutputForTest redirects console log output. Pass nil to restore stderr.
// Does not change an optional LogFile mirror. Intended for tests only.
func SetOutputForTest(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if w == nil {
		out = os.Stderr
		return
	}
	out = w
}

// For returns a Logger for the named component.
// Known components can be disabled via Settings; unknown names always log at the global level.
func For(component string) *Logger {
	mu.Lock()
	enabled, known := components[component]
	if !known {
		enabled = true
	}
	mu.Unlock()

	if !enabled {
		return noop
	}

	return &Logger{component: component, enabled: true}
}

// With returns a child logger that prefixes each message.
func (l *Logger) With(prefix string) *Logger {
	if !l.enabled || prefix == "" {
		return l
	}
	combined := l.prefix
	if combined != "" {
		combined += " "
	}
	combined += prefix
	return &Logger{component: l.component, prefix: combined, enabled: true}
}

// LogDuration logs a warning when elapsed time exceeds the configured slow threshold.
func LogDuration(component, op string, start time.Time) {
	mu.Lock()
	threshold := slowThreshold
	mu.Unlock()
	if threshold <= 0 {
		return
	}
	elapsed := time.Since(start)
	if elapsed < threshold {
		return
	}
	For(component).Warnf("%s took %s (threshold %s)", op, elapsed.Round(time.Millisecond), threshold)
}

func (l *Logger) Tracef(format string, args ...any) { l.log(levelTrace, format, args...) }
func (l *Logger) Debugf(format string, args ...any) { l.log(levelDebug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.log(levelInfo, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.log(levelWarn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.log(levelError, format, args...) }

func (l *Logger) Fatalf(format string, args ...any) {
	l.log(levelError, format, args...)
	os.Exit(1)
}

func (l *Logger) log(lvl level, format string, args ...any) {
	if !l.enabled {
		return
	}

	mu.Lock()
	minLevel := globalLevel
	ts := timestamps
	color := useColor
	w := out
	fw := fileOut
	prefix := l.prefix
	mu.Unlock()

	if lvl < minLevel {
		return
	}

	msg := fmt.Sprintf(format, args...)
	if prefix != "" {
		msg = prefix + " " + msg
	}
	plain := formatLine(lvl, l.component, msg, ts, false)
	console := plain
	if color {
		console = formatLine(lvl, l.component, msg, ts, true)
	}

	_, _ = io.WriteString(w, console)
	if fw != nil {
		_, _ = io.WriteString(fw, plain)
	}
}

func formatLine(lvl level, component, msg string, ts, color bool) string {
	tag := levelTag[lvl]
	comp := padRight(component, 8)

	var b strings.Builder
	if ts {
		b.WriteString(time.Now().Format("2006/01/02 15:04:05"))
		b.WriteByte(' ')
	}

	if color {
		b.WriteString(levelColor[lvl])
	}
	b.WriteString(padRight(tag, 5))
	if color {
		b.WriteString(colorReset)
	}
	b.WriteByte(' ')
	if color {
		b.WriteString(componentColor)
	}
	b.WriteString(comp)
	if color {
		b.WriteString(colorReset)
	}
	b.WriteByte(' ')
	b.WriteString(msg)
	b.WriteByte('\n')
	return b.String()
}

func parseLevel(s string) (level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return levelInfo, nil
	case "trace":
		return levelTrace, nil
	case "debug":
		return levelDebug, nil
	case "warn":
		return levelWarn, nil
	case "error":
		return levelError, nil
	default:
		return 0, fmt.Errorf("logging: invalid level %q (want trace, debug, info, warn, or error)", s)
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}
