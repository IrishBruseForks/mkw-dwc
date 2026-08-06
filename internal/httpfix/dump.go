package httpfix

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	dumpMu   sync.Mutex
	dumpOut  io.Writer
	dumpFile *os.File
)

// SetDumpFile enables raw HTTP recv/send dumps written to path.
// An empty path disables dumps. The file is truncated on open so each
// server start begins with a clean dump. Parent directories are created if missing.
func SetDumpFile(path string) error {
	dumpMu.Lock()
	defer dumpMu.Unlock()

	if dumpFile != nil {
		_ = dumpFile.Close()
		dumpFile = nil
	}
	dumpOut = nil
	if path == "" {
		return nil
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("httpfix: create dump dir %q: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("httpfix: open dump file %q: %w", path, err)
	}
	dumpFile = f
	dumpOut = f
	return nil
}

// SetDumpOutput redirects dump output. Pass nil to disable dumps.
// Intended for tests only.
func SetDumpOutput(w io.Writer) {
	dumpMu.Lock()
	defer dumpMu.Unlock()
	dumpOut = w
}

func dumpEnabled() bool {
	dumpMu.Lock()
	defer dumpMu.Unlock()
	return dumpOut != nil
}

func looksLikeTLS(b []byte) bool {
	// TLS record: ContentType=handshake(0x16), version 0x03 0x00..0x03
	return len(b) >= 3 && b[0] == 0x16 && b[1] == 0x03 && b[2] <= 0x03
}

func formatTrafficVerbose(b []byte) string {
	if len(b) == 0 {
		return "(empty)\n"
	}

	var out strings.Builder
	if mostlyPrintable(b) {
		out.WriteString("text:\n")
		out.WriteString(strconv.Quote(string(b)))
		out.WriteByte('\n')
	}
	out.WriteString("hex:\n")
	out.WriteString(strings.TrimRight(hex.Dump(b), "\n"))
	out.WriteByte('\n')
	return out.String()
}

func mostlyPrintable(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	printable := 0
	for _, c := range b {
		if c == '\t' || c == '\n' || c == '\r' || (c >= 0x20 && c < 0x7f) {
			printable++
		}
	}
	return printable*100/len(b) >= 85
}

func writeDump(component, kind string, addr fmt.Stringer, b []byte) {
	dumpMu.Lock()
	w := dumpOut
	dumpMu.Unlock()
	if w == nil {
		return
	}

	var body strings.Builder
	body.WriteString("=== ")
	body.WriteString(time.Now().Format("2006/01/02 15:04:05.000"))
	body.WriteByte(' ')
	body.WriteString(component)
	body.WriteByte(' ')
	body.WriteString(kind)
	body.WriteByte(' ')
	body.WriteString(addr.String())
	if len(b) > 0 {
		fmt.Fprintf(&body, " (%d bytes)", len(b))
	}
	body.WriteString(" ===\n")
	if len(b) > 0 {
		body.WriteString(formatTrafficVerbose(b))
	} else {
		body.WriteString("(no payload)\n")
	}
	body.WriteByte('\n')

	dumpMu.Lock()
	_, _ = io.WriteString(w, body.String())
	dumpMu.Unlock()
}

func dumpDir(component, dir string, addr fmt.Stringer, b []byte) {
	if !dumpEnabled() || len(b) == 0 {
		return
	}
	writeDump(component, dir, addr, b)
}

func dumpAccept(component string, addr fmt.Stringer) {
	if !dumpEnabled() {
		return
	}
	writeDump(component, "accept", addr, nil)
}

func dumpTLS(component string, addr fmt.Stringer, b []byte) {
	if !dumpEnabled() || len(b) == 0 {
		return
	}
	writeDump(component, "tls", addr, b)
}
