package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type expectedPiRequest struct {
	method   string
	target   string
	body     string
	status   int
	response string
}

func TestPiRuntimeClientRoutes(t *testing.T) {
	t.Parallel()

	expected := []expectedPiRequest{
		{http.MethodGet, "/health", "", http.StatusOK, `{"status":"ok","protocol_version":1,"pi_version":"0.83.0"}`},
		{http.MethodGet, "/v1/providers", "", http.StatusOK, `[{"id":"fixture","name":"Fixture","auth_types":["oauth"],"configured":true,"models":[{"id":"model-1","name":"Model 1","provider_id":"fixture","reasoning":true,"context_window":8192,"max_tokens":1024}]}]`},
		{http.MethodGet, "/v1/credentials", "", http.StatusOK, `[{"provider_id":"fixture","type":"oauth"}]`},
		{http.MethodPost, "/v1/auth/logins", `{"provider_id":"fixture","auth_type":"oauth"}`, http.StatusOK, piLoginFixtureJSON("login-1")},
		{http.MethodGet, "/v1/auth/logins/login-1?after=12", "", http.StatusOK, piLoginFixtureJSON("login-1")},
		{http.MethodPost, "/v1/auth/logins/login-1/responses", `{"prompt_id":"prompt-1","value":"oauth-code"}`, http.StatusNoContent, ""},
		{http.MethodDelete, "/v1/auth/logins/login-1", "", http.StatusNoContent, ""},
		{http.MethodPost, "/v1/auth/logout", `{"provider_id":"fixture"}`, http.StatusNoContent, ""},
		{http.MethodPost, "/v1/legacy-import", `{"base_url":"http://127.0.0.1:11434/v1","model":"legacy-model","api_key":"fixture-api-key","max_tokens":4096}`, http.StatusOK, `{"provider_id":"show-me-the-story-legacy","model_id":"legacy-model","credential_imported":true,"available":true,"made_default":true}`},
		{http.MethodPost, "/v1/sessions/select", `{"project_name":"Story One"}`, http.StatusOK, `{"project_id":"project-1","session_id":"session-1","message_count":3,"restored":false}`},
		{http.MethodGet, "/v1/sessions/current", "", http.StatusOK, `{"project_id":"project-1","session_id":"session-1","message_count":3,"restored":false}`},
	}

	var mu sync.Mutex
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(expected) {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		want := expected[next]
		next++
		if r.Method != want.method || r.URL.RequestURI() != want.target {
			t.Errorf("request %d = %s %s, want %s %s", next, r.Method, r.URL.RequestURI(), want.method, want.target)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		assertJSONTextEqual(t, string(body), want.body)
		if want.body != "" && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(want.status)
		_, _ = w.Write([]byte(want.response))
	}))
	defer server.Close()

	client := newPiRuntimeClient(server.Client(), server.URL)
	ctx := context.Background()

	health, err := client.Health(ctx)
	if err != nil || health.ProtocolVersion != 1 || health.PiVersion != "0.83.0" {
		t.Fatalf("Health() = %#v, %v", health, err)
	}
	providers, err := client.Providers(ctx)
	if err != nil || len(providers) != 1 || providers[0].Models[0].ID != "model-1" {
		t.Fatalf("Providers() = %#v, %v", providers, err)
	}
	credentials, err := client.Credentials(ctx)
	if err != nil || len(credentials) != 1 || credentials[0].Type != "oauth" {
		t.Fatalf("Credentials() = %#v, %v", credentials, err)
	}
	login, err := client.StartLogin(ctx, PiLoginStartRequest{ProviderID: "fixture", AuthType: "oauth"})
	if err != nil || login.ID != "login-1" || login.Prompt == nil || login.Prompt.Type != "manual_code" {
		t.Fatalf("StartLogin() = %#v, %v", login, err)
	}
	login, err = client.LoginStatus(ctx, "login-1", 12)
	if err != nil || login.NextCursor != 4 {
		t.Fatalf("LoginStatus() = %#v, %v", login, err)
	}
	if err := client.RespondLogin(ctx, "login-1", PiLoginResponse{PromptID: "prompt-1", Value: "oauth-code"}); err != nil {
		t.Fatalf("RespondLogin(): %v", err)
	}
	if err := client.CancelLogin(ctx, "login-1"); err != nil {
		t.Fatalf("CancelLogin(): %v", err)
	}
	if err := client.Logout(ctx, "fixture"); err != nil {
		t.Fatalf("Logout(): %v", err)
	}
	legacy, err := client.ImportLegacy(ctx, PiLegacyImportRequest{
		BaseURL: "http://127.0.0.1:11434/v1", Model: "legacy-model", APIKey: "fixture-api-key", MaxTokens: 4096,
	})
	if err != nil || legacy.ModelID != "legacy-model" || !legacy.MadeDefault {
		t.Fatalf("ImportLegacy() = %#v, %v", legacy, err)
	}
	session, err := client.SelectProject(ctx, "Story One")
	if err != nil || session.SessionID != "session-1" {
		t.Fatalf("SelectProject() = %#v, %v", session, err)
	}
	current, err := client.CurrentSession(ctx)
	if err != nil || current == nil || current.ProjectID != "project-1" {
		t.Fatalf("CurrentSession() = %#v, %v", current, err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if next != len(expected) {
		t.Fatalf("received %d requests, want %d", next, len(expected))
	}
}

func TestPiRuntimeClientCurrentSessionNull(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "null")
	}))
	defer server.Close()

	current, err := newPiRuntimeClient(server.Client(), server.URL).CurrentSession(context.Background())
	if err != nil || current != nil {
		t.Fatalf("CurrentSession() = %#v, %v, want nil, nil", current, err)
	}
}

func TestPiRuntimeClientRejectsOversizedAndMalformedResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{"oversized", strings.Repeat("x", 2*1024*1024+1), "exceeds 2 MiB"},
		{"malformed", "{", "decode runtime response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			_, err := newPiRuntimeClient(server.Client(), server.URL).Health(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Health() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPiRuntimeClientHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := newPiRuntimeClient(server.Client(), server.URL).Health(ctx)
		done <- err
	}()
	<-started
	cancel()

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Health() error = %v, want context.Canceled", err)
	}
}

func TestPiRuntimeClientReturnsTypedSecretFreeErrors(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_request","message":"model is invalid"}}`)
	}))
	defer server.Close()

	_, err := newPiRuntimeClient(server.Client(), server.URL).ImportLegacy(context.Background(), PiLegacyImportRequest{
		BaseURL: "http://127.0.0.1:11434/v1", Model: "bad", APIKey: "request-secret",
	})
	var runtimeErr *PiRuntimeError
	if !errors.As(err, &runtimeErr) {
		t.Fatalf("ImportLegacy() error = %T %v, want *PiRuntimeError", err, err)
	}
	if runtimeErr.StatusCode != http.StatusUnprocessableEntity || runtimeErr.Code != "invalid_request" || runtimeErr.Message != "model is invalid" {
		t.Fatalf("runtime error = %#v", runtimeErr)
	}
	if strings.Contains(err.Error(), "request-secret") {
		t.Fatalf("error leaks request secret: %v", err)
	}
}

func piLoginFixtureJSON(id string) string {
	return `{"id":"` + id + `","provider_id":"fixture","auth_type":"oauth","state":"waiting","prompt":{"id":"prompt-1","type":"manual_code","message":"Enter code"},"events":[{"cursor":3,"type":"auth_url","url":"https://example.test/login"}],"next_cursor":4}`
}

func assertJSONTextEqual(t *testing.T, got, want string) {
	t.Helper()
	if want == "" {
		if got != "" {
			t.Errorf("request body = %q, want empty", got)
		}
		return
	}
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Errorf("request body is not JSON: %q: %v", got, err)
		return
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("invalid test JSON %q: %v", want, err)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("request body = %s, want %s", gotJSON, wantJSON)
	}
}
