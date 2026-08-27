package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := WriteFileAtomic(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("content = %q, want second", got)
	}
	matches, err := filepath.Glob(path + ".tmp-*")
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v, err=%v", matches, err)
	}
}

func TestWriteFileAtomicFallsBackToBackupSwap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	ops := systemFileOps
	realRename := ops.rename
	calls := 0
	ops.rename = func(old, new string) error {
		calls++
		if calls == 1 {
			return os.ErrExist
		}
		return realRename(old, new)
	}
	if err := writeFileAtomic(path, []byte("new"), ops); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("content = %q, want new", got)
	}
}

func TestWriteFileAtomicRestoresOriginalWhenInstallFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	ops := systemFileOps
	realRename := ops.rename
	calls := 0
	ops.rename = func(old, new string) error {
		calls++
		switch calls {
		case 1, 3:
			return errors.New("provider rejected move")
		default:
			return realRename(old, new)
		}
	}
	err := writeFileAtomic(path, []byte("new"), ops)
	saveErr, ok := AsSaveError(err)
	if !ok || !saveErr.OriginalPreserved {
		t.Fatalf("expected preserved SaveError, got %#v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Fatalf("content = %q, want restored old", got)
	}
}

func TestWriteFileAtomicKeepsBackupWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	ops := systemFileOps
	realRename := ops.rename
	calls := 0
	ops.rename = func(old, new string) error {
		calls++
		if calls == 2 {
			return realRename(old, new)
		}
		return errors.New("provider rejected move")
	}
	err := writeFileAtomic(path, []byte("new"), ops)
	saveErr, ok := AsSaveError(err)
	if !ok || saveErr.OriginalPreserved || saveErr.BackupPath == "" {
		t.Fatalf("expected retained-backup SaveError, got %#v", err)
	}
	got, readErr := os.ReadFile(saveErr.BackupPath)
	if readErr != nil || string(got) != "old" {
		t.Fatalf("backup content = %q, err=%v", got, readErr)
	}
}
