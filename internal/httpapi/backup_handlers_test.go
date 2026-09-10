package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"showmethestory/internal/config"
	"showmethestory/internal/sse"
	"showmethestory/internal/story"
)

func TestProjectBackupRestoreRoundtrip(t *testing.T) {
	h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
	dir := filepath.Join(h.storysDir(), "source")
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveConfig(filepath.Join(dir, "config.json"), config.DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	state := &story.Progress{Chapters: []story.ChapterState{{Num: 1, Content: "Original prose", Status: story.StatusAccepted}}}
	if err := story.SaveProgress(filepath.Join(dir, "progress.json"), state); err != nil {
		t.Fatal(err)
	}
	if err := story.SaveProjectSettings(filepath.Join(dir, "settings.json"), &story.ProjectSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := story.SavePostProcess(filepath.Join(dir, "postprocess.json"), story.NewProofreadState()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sessions/example.json", "import.json", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"saved":"yes"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(h.progDir, "api.json"), []byte(`{"api_key":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.switchProject("source"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/projects/source/backup", nil)
	request.SetPathValue("name", "source")
	backup := httptest.NewRecorder()
	h.GetProjectBackup(backup, request)
	if backup.Code != 200 || backup.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("backup: %d %s", backup.Code, backup.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(backup.Body.Bytes()), int64(backup.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range zr.File {
		if file.Name == "api.json" {
			t.Fatal("global credentials included")
		}
	}
	restore := httptest.NewRecorder()
	h.PostProjectRestore(restore, httptest.NewRequest("POST", "/api/projects/restore?name=copy", bytes.NewReader(backup.Body.Bytes())))
	if restore.Code != 200 {
		t.Fatalf("restore: %s", restore.Body.String())
	}
	if h.projectName != "source" {
		t.Fatal("restore switched the active project")
	}
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		original, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		copied, err := os.ReadFile(filepath.Join(h.storysDir(), "copy", rel))
		if err != nil {
			return err
		}
		if !bytes.Equal(original, copied) {
			t.Errorf("changed %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.switchProject("copy"); err != nil {
		t.Fatal(err)
	}
	if h.state.Chapters[0].Content != "Original prose" {
		t.Fatal("restored prose missing")
	}
	res := httptest.NewRecorder()
	h.PostProjectRestore(res, httptest.NewRequest("POST", "/api/projects/restore?name=copy", bytes.NewReader(backup.Body.Bytes())))
	if res.Code != 409 {
		t.Fatal("existing project was not protected")
	}
	h.taskRunning = true
	for _, method := range []string{"backup", "restore"} {
		res := httptest.NewRecorder()
		if method == "backup" {
			h.GetProjectBackup(res, request)
		} else {
			h.PostProjectRestore(res, httptest.NewRequest("POST", "/api/projects/restore?name=another", nil))
		}
		if res.Code != 409 {
			t.Fatalf("%s allowed during AI work", method)
		}
	}
}

func TestProjectRestoreRejectsUnsafeOrInvalidArchive(t *testing.T) {
	cfg, err := json.Marshal(config.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, data string
		mode             fs.FileMode
		size             uint64
	}{
		{name: "traversal", path: "../outside.json", data: "{}"},
		{name: "absolute", path: "/outside.json", data: "{}"},
		{name: "windows", path: `C:\outside.json`, data: "{}"},
		{name: "link", path: "link", data: "../outside", mode: os.ModeSymlink | 0777},
		{name: "duplicate", path: "config.json", data: string(cfg)},
		{name: "case collision", path: "CONFIG.JSON", data: string(cfg)},
		{name: "invalid json", path: "settings.json", data: "{"},
		{name: "missing chapter", path: "progress.json", data: `{"chapters":[{"num":1,"status":"accepted"}]}`},
		{name: "rollback", path: "progress.json.rollback", data: "{}"},
		{name: "oversized", path: "large", size: backupFileLimit + 1},
		{name: "legacy", path: "progress.json", data: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandlers(&config.APIConfig{}, "", sse.NewLogBroadcaster(), t.TempDir(), "test")
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			out, err := zw.Create("config.json")
			if err != nil {
				t.Fatal(err)
			}
			configBytes := cfg
			if tc.name == "legacy" {
				configBytes = []byte(`{"project_format_version":3}`)
			}
			if _, err := out.Write(configBytes); err != nil {
				t.Fatal(err)
			}
			header := &zip.FileHeader{Name: tc.path, Method: zip.Store}
			if tc.mode != 0 {
				header.SetMode(tc.mode)
			}
			if tc.size != 0 {
				header.UncompressedSize64 = tc.size
				header.CompressedSize64 = tc.size
				out, err = zw.CreateRaw(header)
			} else {
				out, err = zw.CreateHeader(header)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := out.Write([]byte(tc.data)); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			res := httptest.NewRecorder()
			h.PostProjectRestore(res, httptest.NewRequest("POST", "/api/projects/restore?name=copy", &buf))
			if res.Code != 400 {
				t.Fatalf("unsafe archive accepted: %d %s", res.Code, res.Body.String())
			}
			entries, err := os.ReadDir(h.storysDir())
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("partial restore remains: %v", entries)
			}
		})
	}
	for _, name := range []string{".", "..", "../outside", "a/b", `a\b`, "NUL", "a.", "a\x00b"} {
		if validProjectName(name) {
			t.Errorf("unsafe project name accepted: %q", name)
		}
	}
}

func TestProjectBackupSerializesWithMutations(t *testing.T) {
	entered, release, mutated := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := projectWriteMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/backup") {
			close(entered)
			<-release
		} else if r.Method == "POST" {
			close(mutated)
		}
	}))
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/projects/source/backup", nil))
		close(done)
	}()
	<-entered
	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/projects/select", nil))
	select {
	case <-mutated:
		t.Error("mutation overlapped snapshot")
	case <-time.After(20 * time.Millisecond):
	}
	// Status/SSE GETs stay available during snapshot work.
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/status", nil))
	close(release)
	<-done
	select {
	case <-mutated:
	case <-time.After(time.Second):
		t.Fatal("mutation lock not released")
	}
}
