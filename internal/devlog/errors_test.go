package devlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestErrorMetadataPreservesCausesWithoutPrivateText(t *testing.T) {
	t.Setenv("STORY_DIAGNOSTICS", "")
	dir := t.TempDir()
	Init(dir, "release")
	defer Close()
	var syntax any
	jsonErr := json.Unmarshal([]byte(`{"PRIVATE_FIELD":]`), &syntax)
	for _, tc := range []struct {
		err  error
		kind string
	}{
		{fmt.Errorf("PRIVATE_WRAPPER: %w", context.Canceled), "canceled"},
		{context.DeadlineExceeded, "deadline_exceeded"},
		{&os.PathError{Op: "write", Path: "PRIVATE_PATH", Err: syscall.ENOSPC}, "os_error"},
		{jsonErr, "invalid_json"},
		{errors.New("PRIVATE_MODEL_RESPONSE"), ""},
	} {
		attrs := ErrorAttrs(tc.err)
		Event("warn", "safe_error", attrs...)
		row := map[string]any{}
		for i := 0; i < len(attrs); i += 2 {
			row[attrs[i].(string)] = attrs[i+1]
		}
		if row["error_kind"] != tc.kind && !(tc.kind == "" && row["error_kind"] == nil) {
			t.Fatalf("wrong cause metadata: %v", row)
		}
		if tc.kind == "os_error" && row["errno"] != int(syscall.ENOSPC) {
			t.Fatalf("errno lost: %v", row)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE_") {
		t.Fatal("untrusted error strings leaked")
	}
}
