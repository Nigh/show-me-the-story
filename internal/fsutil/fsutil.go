// Package fsutil provides the small file-write primitives shared by the
// whole app, including defensive writes used for config/progress files.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func Delete(path string) error {
	return os.Remove(path)
}

func Rename(old, new string) error {
	return os.Rename(old, new)
}

// SaveError describes a failed defensive save and whether the previous file
// is still known to be safe. Callers can surface these fields to the user.
type SaveError struct {
	Path              string
	Stage             string
	OriginalPreserved bool
	BackupPath        string
	Err               error
	RestoreErr        error
}

func (e *SaveError) Error() string {
	if e.RestoreErr != nil {
		return fmt.Sprintf("save %s failed during %s: %v; restoring backup failed: %v", e.Path, e.Stage, e.Err, e.RestoreErr)
	}
	return fmt.Sprintf("save %s failed during %s: %v", e.Path, e.Stage, e.Err)
}

func (e *SaveError) Unwrap() error { return e.Err }

// AsSaveError finds a SaveError even when a domain operation wrapped it.
func AsSaveError(err error) (*SaveError, bool) {
	var saveErr *SaveError
	if errors.As(err, &saveErr) {
		return saveErr, true
	}
	return nil, false
}

type fileOps struct {
	createTemp func(string, string) (*os.File, error)
	stat       func(string) (os.FileInfo, error)
	rename     func(string, string) error
	remove     func(string) error
}

var systemFileOps = fileOps{
	createTemp: os.CreateTemp,
	stat:       os.Stat,
	rename:     os.Rename,
	remove:     os.Remove,
}

// WriteFileAtomic first uses the platform's replace-rename operation. Some
// Windows cloud/mapped filesystems reject replacement moves; for those, it
// falls back to a guarded backup swap that preserves the old file whenever
// possible. The fallback is not atomic, but is safer than deleting the target.
func WriteFileAtomic(path string, data []byte) error {
	return writeFileAtomic(path, data, systemFileOps)
}

func writeFileAtomic(path string, data []byte, ops fileOps) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := ops.createTemp(dir, base+".tmp-*")
	if err != nil {
		return &SaveError{Path: path, Stage: "create_temp", OriginalPreserved: true, Err: err}
	}
	tmpPath := tmp.Name()
	tmpExists := true
	defer func() {
		if tmpExists {
			_ = ops.remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return &SaveError{Path: path, Stage: "write_temp", OriginalPreserved: true, Err: err}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return &SaveError{Path: path, Stage: "sync_temp", OriginalPreserved: true, Err: err}
	}
	if err := tmp.Close(); err != nil {
		return &SaveError{Path: path, Stage: "close_temp", OriginalPreserved: true, Err: err}
	}

	if err := ops.rename(tmpPath, path); err == nil {
		tmpExists = false
		return nil
	} else {
		replaceErr := err
		info, statErr := ops.stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return &SaveError{Path: path, Stage: "replace", OriginalPreserved: true, Err: replaceErr}
		}

		backupFile, backupErr := ops.createTemp(dir, base+".bak-*")
		if backupErr != nil {
			return &SaveError{Path: path, Stage: "create_backup", OriginalPreserved: true, Err: backupErr}
		}
		backupPath := backupFile.Name()
		if closeErr := backupFile.Close(); closeErr != nil {
			_ = ops.remove(backupPath)
			return &SaveError{Path: path, Stage: "create_backup", OriginalPreserved: true, Err: closeErr}
		}
		if removeErr := ops.remove(backupPath); removeErr != nil {
			return &SaveError{Path: path, Stage: "create_backup", OriginalPreserved: true, Err: removeErr}
		}

		if backupErr := ops.rename(path, backupPath); backupErr != nil {
			return &SaveError{Path: path, Stage: "backup_original", OriginalPreserved: true, Err: errors.Join(replaceErr, backupErr)}
		}
		if installErr := ops.rename(tmpPath, path); installErr != nil {
			restoreErr := ops.rename(backupPath, path)
			return &SaveError{
				Path:              path,
				Stage:             "install_replacement",
				OriginalPreserved: restoreErr == nil,
				BackupPath:        backupPath,
				Err:               errors.Join(replaceErr, installErr),
				RestoreErr:        restoreErr,
			}
		}

		tmpExists = false
		_ = ops.remove(backupPath) // A stale backup is harmless; the save succeeded.
		return nil
	}
}
