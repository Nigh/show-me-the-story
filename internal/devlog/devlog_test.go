package devlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogNoopWhenNotDev(t *testing.T) {
	mu.Lock()
	enabled, path = false, ""
	mu.Unlock()
	Init(t.TempDir(), "v1.0.0")
	if Enabled() {
		t.Fatal("release version must not enable devlog")
	}
	Log("should not create file")
}

func TestLogWritesTimestamp(t *testing.T) {
	dir := t.TempDir()
	mu.Lock()
	enabled, path = false, ""
	mu.Unlock()
	Init(dir, "dev")
	if !Enabled() {
		t.Fatal("dev version should enable devlog")
	}
	Log("task_start outline_generation")
	data, err := os.ReadFile(filepath.Join(dir, "dev.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "task_start outline_generation") {
		t.Fatalf("missing message: %s", s)
	}
	// YYYY-MM-DD HH:MM:SS.mmm
	if len(s) < 23 || s[4] != '-' || s[10] != ' ' || s[19] != '.' {
		t.Fatalf("missing/invalid timestamp prefix: %q", s)
	}
}
