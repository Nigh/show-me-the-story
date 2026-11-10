package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"showmethestory/internal/devlog"
	"showmethestory/internal/fsutil"
)

func TestDiagnosticTaskIdentityAcrossReservations(t *testing.T) {
	dir := t.TempDir()
	devlog.Init(dir, "v-test")
	defer devlog.Close()
	h := factHandlers(t)
	h.projectName = "PRIVATE_PROJECT_TITLE"
	var taskIDs []string
	for range 2 {
		if !h.tryStartTask() {
			t.Fatal("task reservation failed")
		}
		id := devlog.TaskID(h.taskCtx)
		if id == "" {
			t.Fatal("task context has no diagnostic ID")
		}
		taskIDs = append(taskIDs, id)
		h.skipKnowledgeSync = true
		h.logger.TaskStart("chapter_generation")
		h.logger.TaskEnd("chapter_generation", false)
		h.endTask()
	}
	if taskIDs[0] == taskIDs[1] {
		t.Fatal("separate tasks share diagnostic ID")
	}
	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), h.projectName) {
		t.Fatal("project title leaked into log")
	}
	released := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row["event"] == "task_released" {
			id, _ := row["task_id"].(string)
			if row["running"] != false || row["active_work"] != float64(0) {
				t.Fatalf("task release state lost: %v", row)
			}
			released[id] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, id := range taskIDs {
		if !released[id] {
			t.Fatalf("release missing for task %s", id)
		}
	}
}

func TestHTTPFailureDiagnosticsExcludeRequestAndPanicContent(t *testing.T) {
	dir := t.TempDir()
	devlog.Init(dir, "v-test")
	defer devlog.Close()
	h := factHandlers(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/example/{name}", func(w http.ResponseWriter, r *http.Request) {
		h.writeErrorReq(w, r, http.StatusInternalServerError, "save_failed", &fsutil.SaveError{
			Path: "progress.json", Stage: "replace", OriginalPreserved: true, Err: errors.New("PRIVATE_ERROR_CONTENT"),
		})
	})
	mux.HandleFunc("GET /api/panic/{name}", func(http.ResponseWriter, *http.Request) {
		panic("PRIVATE_PANIC_CONTENT")
	})
	handler := recoveryMiddleware(mux)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/example/PRIVATE_NAME?api_key=PRIVATE_KEY"},
		{"GET", "/api/panic/PRIVATE_NAME"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, strings.NewReader("PRIVATE_REQUEST_BODY")))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", response.Code)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE_") {
		t.Fatal("request, error or panic content leaked")
	}
	foundFailure, foundPanic := false, false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		switch row["event"] {
		case "http_request_failed":
			foundFailure = true
			if row["route"] != "POST /api/example/{name}" || row["stage"] != "replace" || row["original_preserved"] != true {
				t.Fatalf("failure metadata lost: %v", row)
			}
		case "http_panic":
			foundPanic = true
			if row["route"] != "GET /api/panic/{name}" || row["panic_type"] != "string" || row["stack"] == "" {
				t.Fatalf("panic location lost: %v", row)
			}
		}
	}
	if !foundFailure || !foundPanic {
		t.Fatal("HTTP failure diagnostics missing")
	}
}
