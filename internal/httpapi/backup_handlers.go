package httpapi

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
	"showmethestory/internal/story"
)

const (
	backupArchiveLimit = 256 << 20
	backupTotalLimit   = 512 << 20
	backupFileLimit    = 64 << 20
	backupFileCount    = 20000
)

// ponytail: one request gate for backup consistency; snapshot to a separate
// staging file before releasing the gate if large downloads block edits.
// Background AI work is excluded by the task guard; other GETs stay independent.
func projectWriteMiddleware(next http.Handler) http.Handler {
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && (r.Method != http.MethodGet || strings.HasSuffix(r.URL.Path, "/backup")) {
			mu.Lock()
			defer mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func validateBackupProject(dir string) error {
	if err := ensureProjectCompatible(dir); err != nil {
		return err
	}
	if _, err := config.LoadConfig(filepath.Join(dir, "config.json")); err != nil {
		return err
	}
	if _, err := story.LoadProgress(filepath.Join(dir, "progress.json")); err != nil {
		return err
	}
	if _, err := story.LoadProjectSettings(filepath.Join(dir, "settings.json")); err != nil {
		return err
	}
	_, err := story.LoadPostProcess(filepath.Join(dir, "postprocess.json"))
	return err
}

func (h *Handlers) GetProjectBackup(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	name := r.PathValue("name")
	if !validProjectName(name) {
		h.writeErrorReq(w, r, 400, "project_name_invalid_chars")
		return
	}
	dir := filepath.Join(h.storysDir(), name)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		h.writeErrorReq(w, r, 404, "project_not_found")
		return
	}
	// Resolve an interrupted save before taking a snapshot.
	if err := validateBackupProject(dir); err != nil {
		h.writeErrorReq(w, r, 400, "backup_failed", err)
		return
	}
	tmp, err := os.CreateTemp("", "story-backup-*.zip")
	if err != nil {
		h.writeErrorReq(w, r, 500, "backup_failed", err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	zw := zip.NewWriter(tmp)
	var total int64
	count := 0
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := r.Context().Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular file: %s", path)
		}
		total += info.Size()
		count++
		if info.Size() > backupFileLimit || total > backupTotalLimit || count > backupFileCount {
			return fmt.Errorf("backup size limit exceeded")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = filepath.ToSlash(rel), zip.Deflate
		out, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(out, file)
		return err
	})
	closeErr := zw.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		info, err = tmp.Stat()
		if err == nil && info.Size() > backupArchiveLimit {
			err = fmt.Errorf("backup archive size limit exceeded")
		}
	}
	if err != nil {
		h.writeErrorReq(w, r, 500, "backup_failed", err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="project-backup.zip"`)
	http.ServeContent(w, r, "project-backup.zip", time.Time{}, tmp)
}

func (h *Handlers) PostProjectRestore(w http.ResponseWriter, r *http.Request) {
	if h.rejectIfTaskRunning(w, r) {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !validProjectName(name) {
		h.writeErrorReq(w, r, 400, "project_name_invalid_chars")
		return
	}
	dest := filepath.Join(h.storysDir(), name)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		h.writeErrorReq(w, r, 409, "project_exists")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, backupArchiveLimit)
	upload, err := os.CreateTemp("", "story-restore-*.zip")
	if err != nil {
		h.writeErrorReq(w, r, 500, "restore_failed", err)
		return
	}
	defer os.Remove(upload.Name())
	defer upload.Close()
	size, err := io.Copy(upload, r.Body)
	if err != nil {
		h.writeErrorReq(w, r, 400, "restore_failed", err)
		return
	}
	zr, err := zip.NewReader(upload, size)
	if err != nil {
		h.writeErrorReq(w, r, 400, "restore_failed", err)
		return
	}
	if err := os.MkdirAll(h.storysDir(), 0755); err != nil {
		h.writeErrorReq(w, r, 500, "restore_failed", err)
		return
	}
	stage, err := os.MkdirTemp(h.storysDir(), ".restore-")
	if err != nil {
		h.writeErrorReq(w, r, 500, "restore_failed", err)
		return
	}
	defer os.RemoveAll(stage)
	if err := extractProjectBackup(r, zr, stage); err != nil {
		h.writeErrorReq(w, r, 400, "restore_failed", err)
		return
	}
	if err := validateBackupProject(stage); err != nil {
		h.writeErrorReq(w, r, 400, "restore_failed", err)
		return
	}
	if err := r.Context().Err(); err != nil {
		h.writeErrorReq(w, r, 400, "restore_failed", err)
		return
	}
	// Every request that can create/delete projects holds projectWriteMiddleware's lock.
	// Publish only a fully validated directory, without selecting or overwriting a project.
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		h.writeErrorReq(w, r, 409, "project_exists")
		return
	}
	if err := os.Rename(stage, dest); err != nil {
		h.writeErrorReq(w, r, 500, "restore_failed", err)
		return
	}
	h.writeJSON(w, 200, map[string]string{"name": name})
}

func extractProjectBackup(r *http.Request, zr *zip.Reader, dir string) error {
	if len(zr.File) > backupFileCount {
		return fmt.Errorf("too many archive entries")
	}
	seen := map[string]bool{}
	var total uint64
	for _, file := range zr.File {
		if err := r.Context().Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(file.Name, "/")
		local, err := filepath.Localize(name)
		if err != nil || name == "." || strings.ContainsAny(name, "\\:\x00") {
			return fmt.Errorf("invalid archive path: %q", file.Name)
		}
		for _, part := range strings.Split(name, "/") {
			if !validProjectName(part) {
				return fmt.Errorf("invalid archive path: %q", file.Name)
			}
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate archive path: %q", name)
		}
		seen[key] = true
		if file.Mode().IsDir() {
			if err := os.MkdirAll(filepath.Join(dir, local), 0755); err != nil {
				return err
			}
			continue
		}
		if !file.Mode().IsRegular() || strings.HasSuffix(name, ".rollback") {
			return fmt.Errorf("unsupported archive entry: %q", name)
		}
		if file.UncompressedSize64 > backupFileLimit || file.UncompressedSize64 > backupTotalLimit-total {
			return fmt.Errorf("backup size limit exceeded")
		}
		total += file.UncompressedSize64
		in, err := file.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(in, backupFileLimit+1))
		closeErr := in.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if uint64(len(data)) != file.UncompressedSize64 {
			return fmt.Errorf("invalid archive size: %q", name)
		}
		if strings.HasSuffix(name, ".json") && !json.Valid(data) {
			return fmt.Errorf("invalid JSON: %q", name)
		}
		path := filepath.Join(dir, local)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := fsutil.WriteFileAtomic(path, data); err != nil {
			return err
		}
	}
	return nil
}
