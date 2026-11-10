package sse

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"showmethestory/internal/devlog"
	"showmethestory/internal/fsutil"
)

func TestDiagnosticPrivacyAndStorageFailure(t *testing.T) {
	dir := t.TempDir()
	devlog.Init(dir, "v-test")
	defer devlog.Close()
	logger := NewLogBroadcaster()
	defer logger.Close()
	logger.SetDiagnosticContext("task-17", "project-hash")
	logger.TaskStart("knowledge_sync")
	logger.Warn("PRIVATE_NOVEL_BODY")
	logger.WarnKey("test.user_warning", "PRIVATE_NOVEL_NAME")
	logger.ErrorKey("log.knowledge_failed", errors.New("invalid JSON; 原始响应: PRIVATE_MODEL_RESPONSE"))
	logger.ErrorKey("log.knowledge_failed", &fsutil.SaveError{
		Path:  filepath.Join("private-user-directory", "progress.json"),
		Stage: "install_replacement", OriginalPreserved: false,
		BackupPath: filepath.Join("private-user-directory", "progress.json.bak-1"),
		Err:        errors.New("PRIVATE_STORAGE_ERROR"), RestoreErr: errors.New("access denied"),
	})
	logger.TaskEnd("knowledge_sync", false)
	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE_NOVEL_BODY", "PRIVATE_NOVEL_NAME", "PRIVATE_MODEL_RESPONSE", "PRIVATE_STORAGE_ERROR"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("private UI content leaked: %s", secret)
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	foundStorage, foundEnd := false, false
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row["event"] == "storage_error" {
			foundStorage = true
			if row["task_id"] != "task-17" || row["project_id"] != "project-hash" || row["stage"] != "install_replacement" || row["original_preserved"] != false || row["restore_failed"] != true || row["backup_file"] != "progress.json.bak-1" {
				t.Fatalf("storage diagnostics lost: %v", row)
			}
		}
		if row["event"] == "task_end" {
			foundEnd = true
			if row["task_id"] != "task-17" || row["success"] != false {
				t.Fatalf("task result lost: %v", row)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !foundStorage || !foundEnd {
		t.Fatal("missing storage failure or task result")
	}
}
