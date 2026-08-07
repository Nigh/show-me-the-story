// Package devlog writes a local debug log file when the app version is "dev".
// Release builds leave it disabled (no-op). Keep entries sparse: task lifecycle,
// warn/error, and a few coordination events — not stream chunks or step spam.
package devlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	enabled bool
	path    string
)

// Init enables appending to progDir/dev.log when version == "dev".
func Init(progDir, version string) {
	if version != "dev" || progDir == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	enabled = true
	path = filepath.Join(progDir, "dev.log")
	// write unlock-free via unlocked helper while holding mu
	appendLineLocked(fmt.Sprintf("devlog enabled path=%s", path))
}

// Enabled reports whether file logging is on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Log appends one timestamped line. No-op when disabled.
func Log(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return
	}
	appendLineLocked(fmt.Sprintf(format, args...))
}

func appendLineLocked(msg string) {
	line := time.Now().Format("2006-01-02 15:04:05.000") + " " + msg + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}
