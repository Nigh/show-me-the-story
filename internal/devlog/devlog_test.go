package devlog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func readEntries(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func captureStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = original
		reader.Close()
		writer.Close()
	}()
	run()
	writer.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReleaseMetadataAndSafeFields(t *testing.T) {
	t.Setenv("STORY_DIAGNOSTICS", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	Init(dir, "v1.0.0")
	defer Close()
	if !Enabled() || DiagnosticsEnabled() || Path() != path {
		t.Fatal("release metadata logging must be enabled without payload capture")
	}
	RegisterSecret("opaque-credential-value")
	Event("warn", "request_failed",
		"error", errors.New("network failed Authorization: Bearer bearer-value api_key=inline-value https://user:pass@example.com/v1?key=query-value#fragment"),
		"detail", "opaque-credential-value",
		"api_key", "field-value",
		"content", "novel-text",
		"prompt", "prompt-text",
		"messages", []string{"chat-text"},
		"nested", map[string]string{"text": "hidden-text"},
		"long", strings.Repeat("文", 1000),
		"session", "forged")
	RecordPayload(context.Background(), "request-1", "response", "disabled-payload")
	entries := readEntries(t, path)
	start := entries[0]
	if start["event"] != "app_start" || start["version"] != "v1.0.0" || start["os"] != runtime.GOOS || start["arch"] != runtime.GOARCH || start["session"] == "" {
		t.Fatalf("missing startup metadata: %v", start)
	}
	entry := entries[1]
	if !strings.Contains(entry["error"].(string), "network failed") {
		t.Fatal("safe error diagnosis must remain readable")
	}
	if entry["session"] != start["session"] || entry["nested"] != "[unsupported]" || entry["content"] != "[REDACTED]" {
		t.Fatal("reserved fields, nested values, or content were not protected")
	}
	text := entry["long"].(string)
	if len(text) > maxFieldBytes || !utf8.ValidString(text) || !strings.HasSuffix(text, "...[truncated]") {
		t.Fatal("field limit must preserve UTF-8 and mark truncation")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"bearer-value", "inline-value", "query-value", "opaque-credential-value", "field-value", "novel-text", "prompt-text", "chat-text", "hidden-text", "disabled-payload", "user:pass"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("unsafe value leaked: %s", secret)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "diagnostics.log")); !os.IsNotExist(err) {
		t.Fatalf("default must not create payload log: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("log permissions = %o", info.Mode().Perm())
	}
}

func TestPayloadOptInAndTaskCorrelation(t *testing.T) {
	t.Setenv("STORY_DIAGNOSTICS", "true")
	Init(t.TempDir(), "dev")
	defer Close()
	if DiagnosticsEnabled() {
		t.Fatal("only literal STORY_DIAGNOSTICS=1 enables payload capture")
	}
	t.Setenv("STORY_DIAGNOSTICS", "1")
	dir := t.TempDir()
	warning := captureStderr(t, func() { Init(dir, "v2") })
	if !strings.Contains(warning, "sensitive payload capture enabled") {
		t.Fatal("sensitive capture must emit an explicit warning")
	}
	RegisterSecret("opaque-api-value")
	ctx := WithTask(context.Background(), "task-42")
	if TaskID(ctx) != "task-42" || TaskID(context.Background()) != "" || TaskID(nil) != "" {
		t.Fatal("task identity must follow context")
	}
	RecordPayload(ctx, "request-7", "response", "novel-text opaque-api-value "+strings.Repeat("文", maxPayloadBytes))
	entries := readEntries(t, DiagnosticsPath())
	entry := entries[0]
	content := entry["content"].(string)
	if entry["task_id"] != "task-42" || entry["request_id"] != "request-7" || entry["stage"] != "response" || entry["truncated"] != true || entry["sensitive"] != true {
		t.Fatal("missing payload identity or sensitivity metadata")
	}
	if !strings.Contains(content, "novel-text") || strings.Contains(content, "opaque-api-value") || len(content) > maxPayloadBytes || !utf8.ValidString(content) {
		t.Fatal("payload capture must be bounded and redact known credentials")
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "novel-text") {
		t.Fatal("payload must never enter metadata log")
	}
}

func TestBothLogsRotateWithinThreeFiles(t *testing.T) {
	t.Setenv("STORY_DIAGNOSTICS", "1")
	dir := t.TempDir()
	captureStderr(t, func() { Init(dir, "release") })
	defer Close()
	for _, writer := range []*rotatingFile{&app, &payload} {
		for i := range 3 {
			mu.Lock()
			err := writer.file.Truncate(maxFileBytes)
			writer.size = maxFileBytes
			mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if writer == &app {
				Event("info", "rotation_test", "iteration", i)
			} else {
				RecordPayload(context.Background(), "request", "response", "text")
			}
		}
		for _, suffix := range []string{"", ".1", ".2"} {
			info, err := os.Stat(writer.path + suffix)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > maxFileBytes || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
				t.Fatalf("unsafe rotated file %s: size=%d permissions=%o", suffix, info.Size(), info.Mode().Perm())
			}
		}
		if _, err := os.Stat(writer.path + ".3"); !os.IsNotExist(err) {
			t.Fatal("retention must include only current file and two backups")
		}
	}
}

func TestFailuresWarnWithoutStoppingAndRateLimit(t *testing.T) {
	t.Setenv("STORY_DIAGNOSTICS", "")
	dir := t.TempDir()
	warning := captureStderr(t, func() {
		Init(filepath.Join(dir, "missing"), "release")
		Event("error", "first")
		Event("error", "second")
	})
	defer Close()
	if Enabled() || strings.Count(warning, ".open failed:") != 1 {
		t.Fatalf("open failure must warn once without stopping: %s", warning)
	}
	Init(dir, "release")
	warning = captureStderr(t, func() {
		mu.Lock()
		err := app.file.Close()
		mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		Event("info", "write_first")
		Event("info", "write_second")
	})
	if strings.Count(warning, ".write failed:") != 1 {
		t.Fatalf("write failures must warn once: %s", warning)
	}
	Init(dir, "release")
	if err := os.WriteFile(filepath.Join(dir, "app.log.1"), []byte("previous backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "app.log.2"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.log.2", "blocker"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	app.size = maxFileBytes
	mu.Unlock()
	warning = captureStderr(t, func() {
		Event("info", "rotate_first")
		Event("info", "rotate_second")
	})
	if strings.Count(warning, ".rotate failed:") != 1 {
		t.Fatalf("rotation failures must warn once: %s", warning)
	}
}

func TestRotationCannotMoveCurrentPreservesBothBackups(t *testing.T) {
	dir := t.TempDir()
	writer := rotatingFile{path: filepath.Join(dir, "app.log")}
	for suffix, content := range map[string]string{".1": "recent backup", ".2": "oldest backup"} {
		if err := os.WriteFile(writer.path+suffix, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A missing current file deterministically fails the same first rename
	// that fails when Windows locks the current file against moving.
	for range 2 {
		if err := writer.rotate(); err == nil {
			t.Fatal("moving the current file must fail")
		}
		for suffix, want := range map[string]string{".1": "recent backup", ".2": "oldest backup"} {
			data, err := os.ReadFile(writer.path + suffix)
			if err != nil || string(data) != want {
				t.Fatalf("failed rotation changed %s: data=%q error=%v", suffix, data, err)
			}
		}
	}
}

func TestPendingRotationDoesNotDeleteAlreadyMovedBackup(t *testing.T) {
	dir := t.TempDir()
	writer := rotatingFile{path: filepath.Join(dir, "app.log")}
	// Simulate interruption after .1 moved to .2, before .rotate moved to .1.
	for suffix, content := range map[string]string{".rotate": "current log", ".2": "recent backup"} {
		if err := os.WriteFile(writer.path+suffix, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.open(); err != nil {
		t.Fatal(err)
	}
	defer writer.file.Close()
	for suffix, want := range map[string]string{".1": "current log", ".2": "recent backup"} {
		data, err := os.ReadFile(writer.path + suffix)
		if err != nil || string(data) != want {
			t.Fatalf("recovery lost %s: data=%q error=%v", suffix, data, err)
		}
	}
	if _, err := os.Stat(writer.path + ".rotate"); !os.IsNotExist(err) {
		t.Fatal("completed rotation must remove its staging file")
	}
}

func TestPartialWriteRepairsBeforeAppending(t *testing.T) {
	for _, repairFails := range []bool{false, true} {
		name := "immediate_repair"
		if repairFails {
			name = "retry_failed_repair"
		}
		t.Run(name, func(t *testing.T) {
			writer := rotatingFile{path: filepath.Join(t.TempDir(), "app.log")}
			originalPath := writer.path
			if err := writer.open(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if writer.file != nil {
					writer.file.Close()
				}
			}()
			appendLocked(&writer, map[string]any{"event": "before"})
			start := writer.size
			prefix := `{"event":"incomplete`
			n, err := writer.file.WriteString(prefix)
			if err != nil {
				t.Fatal(err)
			}
			if repairFails {
				// An unavailable path prevents repair even while the append handle remains open.
				writer.path += ".missing"
			}
			captureStderr(t, func() {
				writer.finishWrite(start, n, len(prefix)+10, io.ErrUnexpectedEOF)
			})
			if repairFails {
				if !writer.needsRepair || writer.repairOffset != start {
					t.Fatal("failed truncate must retain the pending repair offset")
				}
				captureStderr(t, func() {
					appendLocked(&writer, map[string]any{"event": "blocked"})
				})
				data, err := os.ReadFile(originalPath)
				if err != nil || int64(len(data)) != start+int64(n) {
					t.Fatal("pending repair must block further appends")
				}
				writer.path = originalPath
			}
			appendLocked(&writer, map[string]any{"event": "after"})
			entries := readEntries(t, writer.path)
			if len(entries) != 2 || entries[0]["event"] != "before" || entries[1]["event"] != "after" || writer.needsRepair {
				t.Fatal("partial JSON must be removed before the next complete line")
			}
		})
	}
}
