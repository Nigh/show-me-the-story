// Package devlog writes bounded local JSON diagnostics in every build.
// Event callers must pass metadata, never novel text, prompts, or chat messages.
// STORY_DIAGNOSTICS=1 separately opts into sensitive, bounded payload capture.
package devlog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxFileBytes    = 5 * 1024 * 1024
	maxFieldBytes   = 1024
	maxPayloadBytes = 64 * 1024
	maxAttrs        = 32
	warningInterval = time.Minute
)

type rotatingFile struct {
	path         string
	file         *os.File
	size         int64
	needsRepair  bool
	repairOffset int64
}

var (
	mu                sync.Mutex
	app               rotatingFile
	payload           rotatingFile
	session           string
	secrets           []string
	warnings          = make(map[string]time.Time)
	credentialPattern = regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|password|secret|token)["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;}]+)`)
	bearerPattern     = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/-]+=*`)
	keyPattern        = regexp.MustCompile(`\b(?:sk|rk)-[A-Za-z0-9_-]{8,}`)
	urlPattern        = regexp.MustCompile(`https?://[^\s<>"']+`)
	fieldKeyReplacer  = strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
)

// Init starts app.log even in release builds. Logging failures never stop the app.
// Reinitializing closes previous files; Close releases handles for tests/shutdown.
func Init(progDir, version string) {
	mu.Lock()
	defer mu.Unlock()
	closeLocked()
	app = rotatingFile{}
	payload = rotatingFile{}
	warnings = make(map[string]time.Time)
	secrets = nil
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if sensitiveField(key) && len(value) >= 4 {
			secrets = append(secrets, value)
		}
	}
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	var id [12]byte
	if _, err := rand.Read(id[:]); err == nil {
		session = hex.EncodeToString(id[:])
	} else {
		session = strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	if progDir == "" {
		warnLocked("init", fmt.Errorf("program directory is empty"))
		return
	}
	app.path = filepath.Join(progDir, "app.log")
	if err := app.open(); err != nil {
		warnLocked(app.path+".open", err)
	}
	if os.Getenv("STORY_DIAGNOSTICS") == "1" {
		payload.path = filepath.Join(progDir, "diagnostics.log")
		fmt.Fprintf(os.Stderr, "[diagnostics] WARNING: sensitive payload capture enabled at %s; may contain novel text, prompts and chat. Inspect before sharing.\n", payload.path)
		if err := payload.open(); err != nil {
			warnLocked(payload.path+".open", err)
		}
	}
	eventLocked("info", "app_start", "version", version, "os", runtime.GOOS, "arch", runtime.GOARCH, "diagnostics", payload.file != nil)
}

// Close releases diagnostic files. Calling it repeatedly is safe.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	closeLocked()
	app.path = ""
	payload.path = ""
}

func closeLocked() {
	for _, writer := range []*rotatingFile{&app, &payload} {
		if writer.file != nil {
			if err := writer.file.Close(); err != nil {
				warnLocked(writer.path+".close", err)
			}
			writer.file = nil
		}
	}
}

// Enabled reports whether the metadata log is currently open.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return app.file != nil
}

// Path returns the metadata log's configured path, even if opening it failed.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return app.path
}

// DiagnosticsEnabled reports whether sensitive capture was explicitly enabled.
func DiagnosticsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return payload.path != ""
}

// DiagnosticsPath returns the separate sensitive log path, or "" when disabled.
func DiagnosticsPath() string {
	mu.Lock()
	defer mu.Unlock()
	return payload.path
}

// RegisterSecret keeps an API credential out of all subsequent log output.
// Call before logging a request; registered secrets are never written to disk.
func RegisterSecret(secret string) {
	if secret == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range secrets {
		if existing == secret {
			return
		}
	}
	secrets = append(secrets, secret)
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
}

// Event writes bounded metadata. Nested values are rejected rather than serialized.
// Sensitive field names are redacted; callers must not disguise content as metadata.
func Event(level, event string, attrs ...any) {
	mu.Lock()
	defer mu.Unlock()
	eventLocked(level, event, attrs...)
}

func eventLocked(level, event string, attrs ...any) {
	if app.path == "" {
		return
	}
	entry := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
		"level":   safeText(level, maxFieldBytes),
		"event":   safeText(event, maxFieldBytes),
		"session": session,
	}
	for i := 0; i+1 < len(attrs) && i < maxAttrs*2; i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			continue
		}
		switch key {
		case "time", "level", "event", "session":
			continue
		}
		value := attrs[i+1]
		if sensitiveField(key) {
			value = "[REDACTED]"
		} else {
			value = safeValue(value)
		}
		entry[safeText(key, maxFieldBytes)] = value
	}
	appendLocked(&app, entry)
}

type taskKey struct{}

// WithTask associates a real application task ID with its existing context.
func WithTask(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, taskKey{}, id)
}

// TaskID returns the associated task ID, or "" outside a task.
func TaskID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(taskKey{}).(string)
	return id
}

// RecordPayload writes content only after explicit STORY_DIAGNOSTICS=1 opt-in.
// Payloads remain sensitive after credential redaction; never share unchecked.
func RecordPayload(ctx context.Context, requestID, stage, content string) {
	mu.Lock()
	defer mu.Unlock()
	if payload.path == "" {
		return
	}
	appendLocked(&payload, map[string]any{
		"time":          time.Now().UTC().Format(time.RFC3339Nano),
		"session":       session,
		"task_id":       safeText(TaskID(ctx), maxFieldBytes),
		"request_id":    safeText(requestID, maxFieldBytes),
		"stage":         safeText(stage, maxFieldBytes),
		"content":       safeText(content, maxPayloadBytes),
		"content_bytes": len(content),
		"truncated":     len(content) > maxPayloadBytes,
		"sensitive":     true,
	})
}

func safeValue(value any) any {
	switch value := value.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return value
	case float32:
		if !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) {
			return value
		}
	case float64:
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value
		}
	case string:
		return safeText(value, maxFieldBytes)
	case error:
		return safeText(value.Error(), maxFieldBytes)
	}
	return "[unsupported]"
}

func sensitiveField(key string) bool {
	key = strings.ToLower(key)
	key = fieldKeyReplacer.Replace(key)
	for _, part := range []string{"apikey", "authorization", "password", "secret", "credential", "cookie", "accesstoken", "refreshtoken"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	for _, suffix := range []string{"bytes", "chars", "count", "size", "tokens", "rev", "hash", "id"} {
		if strings.HasSuffix(key, suffix) {
			return false
		}
	}
	for _, part := range []string{"content", "prompt", "chat", "messages", "body", "payload", "rawresponse", "rawrequest"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	switch key {
	case "content", "prompt", "prompts", "chat", "messages", "body", "requestbody", "responsebody", "raw", "response", "rawresponse", "payload", "token":
		return true
	}
	return false
}

func safeText(text string, limit int) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	text = bearerPattern.ReplaceAllString(text, "Bearer [REDACTED]")
	text = credentialPattern.ReplaceAllString(text, "${1}[REDACTED]")
	text = keyPattern.ReplaceAllString(text, "[REDACTED]")
	text = urlPattern.ReplaceAllStringFunc(text, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[REDACTED_URL]"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	})
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	if len(text) <= limit {
		return text
	}
	end := limit - len("...[truncated]")
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "...[truncated]"
}

func appendLocked(writer *rotatingFile, entry map[string]any) {
	data, err := json.Marshal(entry)
	if err != nil {
		warnLocked(writer.path+".encode", err)
		return
	}
	data = append(data, '\n')
	if writer.file == nil {
		if err := writer.open(); err != nil {
			warnLocked(writer.path+".open", err)
			return
		}
	}
	if err := writer.repair(); err != nil {
		warnLocked(writer.path+".repair", err)
		return
	}
	if writer.size+int64(len(data)) > maxFileBytes {
		if err := writer.rotate(); err != nil {
			warnLocked(writer.path+".rotate", err)
			return
		}
	}
	start := writer.size
	n, err := writer.file.Write(data)
	writer.finishWrite(start, n, len(data), err)
}

// A failed partial write must be removed before another JSON line is appended.
func (writer *rotatingFile) finishWrite(start int64, n, expected int, err error) {
	writer.size = start + int64(n)
	if err == nil && n == expected {
		return
	}
	if n > 0 && n < expected {
		writer.needsRepair = true
		writer.repairOffset = start
		if repairErr := writer.repair(); repairErr != nil {
			warnLocked(writer.path+".repair", repairErr)
		}
	}
	if err == nil {
		err = fmt.Errorf("short write: %d/%d bytes", n, expected)
	}
	warnLocked(writer.path+".write", err)
}

func (writer *rotatingFile) repair() error {
	if !writer.needsRepair {
		return nil
	}
	if err := os.Truncate(writer.path, writer.repairOffset); err != nil {
		return err
	}
	writer.size = writer.repairOffset
	writer.needsRepair = false
	return nil
}

func (writer *rotatingFile) open() error {
	if err := writer.recoverRotation(); err != nil {
		return err
	}
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *rotatingFile) rotate() error {
	if writer.file != nil {
		err := writer.file.Close()
		writer.file = nil
		if err != nil {
			return err
		}
	}
	// Park the current file first. A Windows sharing violation must leave both
	// backups untouched. The parked file also records recovery state on disk.
	if _, err := os.Lstat(writer.path + ".rotate"); err == nil {
		return writer.open()
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(writer.path, writer.path+".rotate"); err != nil {
		return err
	}
	return writer.open()
}

func (writer *rotatingFile) recoverRotation() error {
	info, err := os.Lstat(writer.path + ".rotate")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("rotation staging path is not a regular file")
	}
	if _, err := os.Lstat(writer.path + ".1"); err == nil {
		if err := os.Remove(writer.path + ".2"); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(writer.path+".1", writer.path+".2"); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Missing .1 means it was already moved, so a retry must not delete .2.
	return os.Rename(writer.path+".rotate", writer.path+".1")
}

func warnLocked(operation string, err error) {
	now := time.Now()
	if previous, ok := warnings[operation]; ok && now.Sub(previous) < warningInterval {
		return
	}
	warnings[operation] = now
	fmt.Fprintf(os.Stderr, "[diagnostics] %s failed: %s; logging may be incomplete (repeats limited to once/minute).\n", safeText(operation, maxFieldBytes), safeText(err.Error(), maxFieldBytes))
}
