package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Settings configures global logging behavior.
type Settings struct {
	Level      string // debug, info, warn, error (default info)
	Color      string // auto, always, never (default auto)
	Timestamps bool   // default true
	Nas        bool   // default true
	Profile    bool
	Qr         bool
	Browser    bool
	Natneg     bool
	Proxy      bool
	App        bool
}

type level int

const (
	levelDebug level = iota
	levelInfo
	levelWarn
	levelError
)

// Logger writes leveled messages for a single component.
type Logger struct {
	component string
	enabled   bool
}

var (
	mu          sync.Mutex
	globalLevel = levelInfo
	useColor    bool
	timestamps  = true
	components  = map[string]bool{
		"nas":     true,
		"profile": true,
		"qr":      true,
		"browser": true,
		"natneg":  true,
		"proxy":   true,
		"app":     true,
	}
	out io.Writer = os.Stderr

	noop = &Logger{enabled: false}

	levelTag = map[level]string{
		levelDebug: "DEBUG",
		levelInfo:  "INFO",
		levelWarn:  "WARN",
		levelError: "ERROR",
	}

	levelColor = map[level]string{
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
	switch color {
	case "auto":
		useColor = stderrIsTerminal()
	case "always":
		useColor = true
	case "never":
		useColor = false
	default:
		return fmt.Errorf("logging: invalid color %q (want auto, always, or never)", s.Color)
	}

	mu.Lock()
	defer mu.Unlock()

	globalLevel = lvl
	timestamps = s.Timestamps
	components["nas"] = s.Nas
	components["profile"] = s.Profile
	components["qr"] = s.Qr
	components["browser"] = s.Browser
	components["natneg"] = s.Natneg
	components["proxy"] = s.Proxy
	components["app"] = s.App

	return nil
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
	mu.Unlock()

	if lvl < minLevel {
		return
	}

	tag := levelTag[lvl]
	comp := padRight(l.component, 8)
	msg := fmt.Sprintf(format, args...)

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

	mu.Lock()
	_, _ = io.WriteString(out, b.String())
	mu.Unlock()
}

func parseLevel(s string) (level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return levelInfo, nil
	case "debug":
		return levelDebug, nil
	case "warn":
		return levelWarn, nil
	case "error":
		return levelError, nil
	default:
		return 0, fmt.Errorf("logging: invalid level %q (want debug, info, warn, or error)", s)
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
