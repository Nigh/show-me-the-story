package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/story"
	"strings"
	"testing"
)

func TestProjectSelectPreservesCurrentProjectOnStorageFailure(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, recovery := range []bool{false, true} {
			t.Run(lang+fmt.Sprint(recovery), func(t *testing.T) {
				root := t.TempDir()
				writeProjectFile(t, root, "storys/broken/config.json", `{"project_format_version":4}`)
				writeProjectFile(t, root, "storys/broken/progress.json", `{"chapters":[{"num":1,"status":"accepted","word_count":10}]}`)
				if recovery {
					writeProjectFile(t, root, "storys/broken/progress.json.rollback", `{`)
				}
				h := NewHandlers(nil, "", nil, root, "test")
				h.projectName = "current"
				before := h.state
				r := httptest.NewRequest("POST", "/api/projects/select", strings.NewReader(`{"name":"broken"}`))
				r.Header.Set("X-UI-Locale", lang)
				res := httptest.NewRecorder()
				h.PostProjectSelect(res, r)
				if res.Code != http.StatusBadRequest || h.projectName != "current" || h.state != before {
					t.Fatal("failed load switched project", res.Code)
				}
				var response map[string]any
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if recovery {
					if response["code"] != "storage_save_failed" || response["storage_error"] == nil {
						t.Fatal("recovery error lost diagnostics", response)
					}
				} else {
					message := response["error"].(string)
					want := "第 1 章"
					if lang == "en" {
						want = "Chapter 1"
					}
					if !strings.Contains(message, want) || !strings.Contains(message, filepath.Join(root, "storys", "broken", "chapters", "000001.json")) {
						t.Fatal("missing localized chapter diagnostic", message)
					}
				}
			})
		}
	}
}

func TestDetectProjectCompatibility(t *testing.T) {
	root := t.TempDir()

	if got := config.DefaultConfig().ProjectFormatVersion; got != config.ProjectFormatVersion {
		t.Fatalf("new config format version = %d, want %d", got, config.ProjectFormatVersion)
	}
	writeProjectFile(t, root, "marked/config.json", `{"project_format_version":4}`)
	if got, err := detectProjectCompatibility(filepath.Join(root, "marked")); err != nil || got != projectCompatibilitySupported {
		t.Fatalf("marked project = %q, %v; want supported, nil", got, err)
	}

	v3Config := `{"project_format_version":3,"language":"zh"}`
	writeProjectFile(t, root, "v3/config.json", v3Config)
	if got, err := detectProjectCompatibility(filepath.Join(root, "v3")); err != nil || got != projectCompatibilityV3 {
		t.Fatalf("v3 project = %q, %v; want v3 incompatible, nil", got, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "v3/config.json"))
	if err != nil || string(data) != v3Config {
		t.Fatalf("v3 config changed during detection: %q, %v", data, err)
	}

	writeProjectFile(t, root, "v2/config.json", `{"project_format_version":2}`)
	if got, err := detectProjectCompatibility(filepath.Join(root, "v2")); err != nil || got != projectCompatibilityLegacy {
		t.Fatalf("v2 project = %q, %v; want legacy incompatible, nil", got, err)
	}

	writeProjectFile(t, root, "unmarked-v2/config.json", `{"language":"zh"}`)
	writeProjectFile(t, root, "unmarked-v2/progress.json", `{"chapters":[{"num":1,"content":"旧版正文"}]}`)
	if got, err := detectProjectCompatibility(filepath.Join(root, "unmarked-v2")); err != nil || got != projectCompatibilityLegacy {
		t.Fatalf("unmarked v2 project = %q, %v; want legacy incompatible, nil", got, err)
	}

	writeProjectFile(t, root, "unmarked-v3/config.json", `{"language":"zh"}`)
	writeProjectFile(t, root, "unmarked-v3/progress.json", `{"chapters":[{"num":1}]}`)
	writeProjectFile(t, root, "unmarked-v3/chapters/000001.json", `{"num":1,"content":"正文"}`)
	if got, err := detectProjectCompatibility(filepath.Join(root, "unmarked-v3")); err != nil || got != projectCompatibilityV3 {
		t.Fatalf("unmarked v3 project = %q, %v; want v3 incompatible, nil", got, err)
	}

	writeProjectFile(t, root, "unknown/config.json", `{"language":"zh"}`)
	if got, err := detectProjectCompatibility(filepath.Join(root, "unknown")); err != nil || got != projectCompatibilityUnknown {
		t.Fatalf("unmarked empty project = %q, %v; want unknown, nil", got, err)
	}
}

func TestCompatibilityVersions(t *testing.T) {
	tests := []struct {
		kind, format, line, recommended string
	}{
		{projectCompatibilityV3, "v3", "v3.0.x", "v3.0.3"},
		{projectCompatibilityLegacy, "v2", "v2.x", "v2.5.2"},
		{projectCompatibilityUnknown, "unknown", "", ""},
	}
	for _, tt := range tests {
		format, line, recommended := compatibilityVersions(tt.kind)
		if format != tt.format || line != tt.line || recommended != tt.recommended {
			t.Fatalf("compatibilityVersions(%q) = %q, %q, %q", tt.kind, format, line, recommended)
		}
	}
}

func TestGetProjectsUsesConfiguredTitleAndCompletedStatus(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "storys/done/config.json", `{"project_format_version":4,"language":"zh","story":{"title":"配置书名"}}`)
	writeProjectFile(t, root, "storys/done/progress.json", `{"phase":"writing","title":"","book_status":"completed","chapters":[]}`)

	h := NewHandlers(nil, "", nil, root, "test")
	response := httptest.NewRecorder()
	h.GetProjects(response, httptest.NewRequest(http.MethodGet, "/api/projects", nil))

	var projects []map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0]["title"] != "配置书名" || projects[0]["book_status"] != story.BookStatusCompleted {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestProjectSelectRejectsLegacyBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "storys/legacy/config.json", `{"language":"zh"}`)
	writeProjectFile(t, root, "storys/legacy/progress.json", `{"chapters":[{"num":1,"content":"旧版正文"}]}`)

	h := NewHandlers(nil, "", nil, root, "test")
	request := httptest.NewRequest(http.MethodPost, "/api/projects/select", bytes.NewBufferString(`{"name":"legacy"}`))
	response := httptest.NewRecorder()
	h.PostProjectSelect(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	if _, err := os.Stat(filepath.Join(root, "storys/legacy/sessions")); !os.IsNotExist(err) {
		t.Fatalf("legacy project was modified: sessions directory error = %v", err)
	}
}

func writeProjectFile(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
