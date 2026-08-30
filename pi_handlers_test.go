package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const piHandlerSecret = "pi-handler-private-key"

func TestPiRoutesForwardFoundationAPIWithoutMutatingFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	apiPath := filepath.Join(root, "api.json")
	apiConfig := &APIConfig{
		APIKey: piHandlerSecret, BaseURL: "http://127.0.0.1:11434/v1", Model: "legacy-model", MaxTokens: 4096,
	}
	if err := saveAPIConfig(apiPath, apiConfig); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(root, "storys", "Story One")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "marker.txt"), []byte("canonical"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeFiles := snapshotTestTree(t, root)

	runtime := newRecordingPiRuntime()
	logger := NewLogBroadcaster()
	logs := logger.Subscribe()
	h := NewHandlers(apiConfig, apiPath, logger, root, "test")
	h.projectName = "Story One"
	h.SetPiRuntime(runtime)
	mux := http.NewServeMux()
	registerPiRoutes(mux, h)

	responses := []*httptest.ResponseRecorder{
		piRouteRequest(mux, http.MethodGet, "/api/pi/status", nil),
		piRouteRequest(mux, http.MethodGet, "/api/pi/providers", nil),
		piRouteRequest(mux, http.MethodGet, "/api/pi/credentials", nil),
		piRouteRequest(mux, http.MethodPost, "/api/pi/auth/login", map[string]any{"provider_id": "fixture", "auth_type": "oauth"}),
		piRouteRequest(mux, http.MethodGet, "/api/pi/auth/login/login-1?after=2", nil),
		piRouteRequest(mux, http.MethodPost, "/api/pi/auth/login/login-1/respond", map[string]any{"prompt_id": "prompt-1", "value": piHandlerSecret}),
		piRouteRequest(mux, http.MethodDelete, "/api/pi/auth/login/login-1", nil),
		piRouteRequest(mux, http.MethodPost, "/api/pi/auth/logout", map[string]any{"provider_id": "fixture"}),
		piRouteRequest(mux, http.MethodPost, "/api/pi/import-legacy-api/preview", nil),
		piRouteRequest(mux, http.MethodPost, "/api/pi/import-legacy-api", nil),
		piRouteRequest(mux, http.MethodPost, "/api/pi/session/select", nil),
		piRouteRequest(mux, http.MethodGet, "/api/pi/session/current", nil),
	}
	wantStatuses := []int{200, 200, 200, 202, 200, 204, 204, 204, 200, 200, 200, 200}
	for index, response := range responses {
		if response.Code != wantStatuses[index] {
			t.Errorf("response %d status = %d, want %d: %s", index, response.Code, wantStatuses[index], response.Body.String())
		}
		if strings.Contains(response.Body.String(), piHandlerSecret) {
			t.Errorf("response %d leaked API key: %s", index, response.Body.String())
		}
	}

	var preview map[string]any
	if err := json.Unmarshal(responses[8].Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["provider_id"] != "show-me-the-story-legacy" || preview["model"] != "legacy-model" || preview["base_url"] != resolveAPIBase(apiConfig.BaseURL, apiConfig.URLStrict) || preview["max_tokens"] != float64(4096) {
		t.Fatalf("preview = %#v", preview)
	}
	if _, exists := preview["api_key"]; exists {
		t.Fatalf("preview contains api_key: %#v", preview)
	}

	runtime.mu.Lock()
	if runtime.imported.APIKey != piHandlerSecret {
		t.Errorf("runtime import API key = %q", runtime.imported.APIKey)
	}
	if runtime.selected != "Story One" {
		t.Errorf("runtime selected project = %q", runtime.selected)
	}
	if runtime.loginAfter != 2 || runtime.loginResponse.Value != piHandlerSecret {
		t.Errorf("login forwarding = after %d, response %#v", runtime.loginAfter, runtime.loginResponse)
	}
	runtime.mu.Unlock()

	afterFiles := snapshotTestTree(t, root)
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Fatalf("Pi routes modified files\nbefore: %#v\nafter:  %#v", beforeFiles, afterFiles)
	}
	logger.Close()
	for message := range logs {
		if strings.Contains(fmt.Sprint(message.Data), piHandlerSecret) {
			t.Fatalf("logger leaked API key: %#v", message)
		}
	}
}

func TestPiHandlerValidationAndUnavailableMapping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	apiPath := filepath.Join(root, "api.json")
	apiConfig := &APIConfig{APIKey: piHandlerSecret, BaseURL: "file:///tmp/model", Model: ""}
	if err := saveAPIConfig(apiPath, apiConfig); err != nil {
		t.Fatal(err)
	}
	runtime := newRecordingPiRuntime()
	h := NewHandlers(apiConfig, apiPath, NewLogBroadcaster(), root, "test")
	h.SetPiRuntime(runtime)
	mux := http.NewServeMux()
	registerPiRoutes(mux, h)
	mux.HandleFunc("GET /api/config/api", h.GetAPIConfig)

	if response := piRouteRequest(mux, http.MethodPost, "/api/pi/session/select", nil); response.Code != http.StatusConflict {
		t.Fatalf("session select without project = %d: %s", response.Code, response.Body.String())
	}
	if response := piRouteRequest(mux, http.MethodPost, "/api/pi/import-legacy-api/preview", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid preview = %d: %s", response.Code, response.Body.String())
	}
	if response := piRouteRequest(mux, http.MethodPost, "/api/pi/auth/login", map[string]any{"provider_id": "fixture", "auth_type": "oauth", "extra": true}); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown login field = %d: %s", response.Code, response.Body.String())
	}
	large := httptest.NewRequest(http.MethodPost, "/api/pi/auth/login", bytes.NewReader(bytes.Repeat([]byte("x"), 1024*1024+1)))
	large.RemoteAddr = "127.0.0.1:12345"
	large.Host = "local.test"
	largeResponse := httptest.NewRecorder()
	mux.ServeHTTP(largeResponse, large)
	if largeResponse.Code != http.StatusBadRequest {
		t.Fatalf("oversized login = %d: %s", largeResponse.Code, largeResponse.Body.String())
	}

	runtime.mu.Lock()
	runtime.err = ErrPiRuntimeUnavailable
	runtime.mu.Unlock()
	if response := piRouteRequest(mux, http.MethodGet, "/api/pi/providers", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable providers = %d: %s", response.Code, response.Body.String())
	}
	if response := piRouteRequest(mux, http.MethodGet, "/api/pi/status", nil); response.Code != http.StatusOK {
		t.Fatalf("unavailable status = %d: %s", response.Code, response.Body.String())
	}
	if response := piRouteRequest(mux, http.MethodGet, "/api/config/api", nil); response.Code != http.StatusOK {
		t.Fatalf("legacy config while Pi unavailable = %d: %s", response.Code, response.Body.String())
	}

	runtime.mu.Lock()
	runtime.err = &PiRuntimeError{StatusCode: http.StatusNotFound, Code: "login_not_found", Message: "missing"}
	runtime.mu.Unlock()
	if response := piRouteRequest(mux, http.MethodGet, "/api/pi/auth/login/missing", nil); response.Code != http.StatusNotFound {
		t.Fatalf("missing login = %d: %s", response.Code, response.Body.String())
	}
}

func TestPiHandlerCurrentSessionNoContent(t *testing.T) {
	t.Parallel()
	runtime := newRecordingPiRuntime()
	runtime.current = nil
	h := NewHandlers(DefaultAPIConfig(), "", NewLogBroadcaster(), t.TempDir(), "test")
	h.SetPiRuntime(runtime)
	mux := http.NewServeMux()
	registerPiRoutes(mux, h)

	response := piRouteRequest(mux, http.MethodGet, "/api/pi/session/current", nil)
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("current session = %d %q", response.Code, response.Body.String())
	}
}

func TestPiRoutesRejectRemoteOrMismatchedOriginBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := newRecordingPiRuntime()
	h := NewHandlers(DefaultAPIConfig(), "", NewLogBroadcaster(), t.TempDir(), "test")
	h.SetPiRuntime(runtime)
	mux := http.NewServeMux()
	registerPiRoutes(mux, h)

	tests := []struct {
		name       string
		remoteAddr string
		origin     string
		want       int
	}{
		{"remote", "203.0.113.10:1234", "", http.StatusForbidden},
		{"origin mismatch", "127.0.0.1:1234", "http://evil.test", http.StatusForbidden},
		{"loopback no origin", "127.0.0.1:1234", "", http.StatusOK},
		{"loopback matching origin", "[::1]:1234", "http://local.test", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/pi/providers", nil)
			request.RemoteAddr = tc.remoteAddr
			request.Host = "local.test"
			request.Header.Set("Origin", tc.origin)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
		})
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.calls != 2 {
		t.Fatalf("runtime calls = %d, want only the two allowed requests", runtime.calls)
	}
}

func TestPiRoutesGuardPreflightBeforeCORS(t *testing.T) {
	t.Parallel()
	h := NewHandlers(DefaultAPIConfig(), "", NewLogBroadcaster(), t.TempDir(), "test")
	h.SetPiRuntime(newRecordingPiRuntime())
	mux := http.NewServeMux()
	registerPiRoutes(mux, h)
	handler := corsMiddleware(mux)

	tests := []struct {
		name       string
		remoteAddr string
		origin     string
		want       int
	}{
		{"remote", "203.0.113.10:1234", "http://local.test", http.StatusForbidden},
		{"origin mismatch", "127.0.0.1:1234", "http://evil.test", http.StatusForbidden},
		{"allowed", "127.0.0.1:1234", "http://local.test", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "/api/pi/providers", nil)
			request.RemoteAddr = tc.remoteAddr
			request.Host = "local.test"
			request.Header.Set("Origin", tc.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
		})
	}
}

func piRouteRequest(handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:12345"
	request.Host = "local.test"
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func snapshotTestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type recordingPiRuntime struct {
	mu            sync.Mutex
	err           error
	calls         int
	imported      PiLegacyImportRequest
	selected      string
	loginAfter    int
	loginResponse PiLoginResponse
	current       *PiSessionSummary
}

func newRecordingPiRuntime() *recordingPiRuntime {
	return &recordingPiRuntime{current: &PiSessionSummary{ProjectID: "project-1", SessionID: "session-1"}}
}

func (runtime *recordingPiRuntime) record() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.calls++
	return runtime.err
}

func (runtime *recordingPiRuntime) Status() PiRuntimeStatus {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return PiRuntimeStatus{Available: runtime.err == nil, Pi: "0.83.0"}
}

func (runtime *recordingPiRuntime) Health(context.Context) (PiHealth, error) {
	if err := runtime.record(); err != nil {
		return PiHealth{}, err
	}
	return PiHealth{Status: "ok", ProtocolVersion: 1, PiVersion: "0.83.0"}, nil
}
func (runtime *recordingPiRuntime) Providers(context.Context) ([]PiProvider, error) {
	if err := runtime.record(); err != nil {
		return nil, err
	}
	return []PiProvider{{ID: "fixture", Name: "Fixture"}}, nil
}
func (runtime *recordingPiRuntime) Credentials(context.Context) ([]PiCredential, error) {
	if err := runtime.record(); err != nil {
		return nil, err
	}
	return []PiCredential{{ProviderID: "fixture", Type: "oauth"}}, nil
}
func (runtime *recordingPiRuntime) StartLogin(_ context.Context, request PiLoginStartRequest) (PiLoginSnapshot, error) {
	if err := runtime.record(); err != nil {
		return PiLoginSnapshot{}, err
	}
	return PiLoginSnapshot{ID: "login-1", ProviderID: request.ProviderID, AuthType: request.AuthType, State: "running", Events: []PiAuthEvent{}}, nil
}
func (runtime *recordingPiRuntime) LoginStatus(_ context.Context, id string, after int) (PiLoginSnapshot, error) {
	if err := runtime.record(); err != nil {
		return PiLoginSnapshot{}, err
	}
	runtime.mu.Lock()
	runtime.loginAfter = after
	runtime.mu.Unlock()
	return PiLoginSnapshot{ID: id, State: "waiting", Events: []PiAuthEvent{}}, nil
}
func (runtime *recordingPiRuntime) RespondLogin(_ context.Context, _ string, response PiLoginResponse) error {
	if err := runtime.record(); err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.loginResponse = response
	runtime.mu.Unlock()
	return nil
}
func (runtime *recordingPiRuntime) CancelLogin(context.Context, string) error {
	return runtime.record()
}
func (runtime *recordingPiRuntime) Logout(context.Context, string) error { return runtime.record() }
func (runtime *recordingPiRuntime) ImportLegacy(_ context.Context, request PiLegacyImportRequest) (PiLegacyImportResult, error) {
	if err := runtime.record(); err != nil {
		return PiLegacyImportResult{}, err
	}
	runtime.mu.Lock()
	runtime.imported = request
	runtime.mu.Unlock()
	return PiLegacyImportResult{ProviderID: "show-me-the-story-legacy", ModelID: request.Model, CredentialImported: true, Available: true, MadeDefault: true}, nil
}
func (runtime *recordingPiRuntime) SelectProject(_ context.Context, name string) (PiSessionSummary, error) {
	if err := runtime.record(); err != nil {
		return PiSessionSummary{}, err
	}
	runtime.mu.Lock()
	runtime.selected = name
	runtime.mu.Unlock()
	return PiSessionSummary{ProjectID: "project-1", SessionID: "session-1"}, nil
}
func (runtime *recordingPiRuntime) CurrentSession(context.Context) (*PiSessionSummary, error) {
	if err := runtime.record(); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.current, nil
}
func (runtime *recordingPiRuntime) Close() error { return nil }

var _ PiRuntime = (*recordingPiRuntime)(nil)
