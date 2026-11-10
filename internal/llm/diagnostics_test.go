package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"showmethestory/internal/config"
	"showmethestory/internal/devlog"
)

func TestAPIStatusErrorsNeverExposeBody(t *testing.T) {
	const secret = "provider-echoed-prompt-and-api-key"
	for _, status := range []int{400, 401, 403, 404, 429, 500} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/stream=%t", status, stream), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					fmt.Fprintf(w, `{"error":{"message":%q,"code":%q,"type":%q}}`, secret, secret, secret)
				}))
				defer server.Close()
				cfg := &config.APIConfig{BaseURL: server.URL, Model: "test", APIKey: secret}
				var err error
				if stream {
					_, err = CallAPIStreamMessages(context.Background(), cfg, nil, nil)
				} else {
					_, err = CallAPIMessagesSync(context.Background(), cfg, nil)
				}
				if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), fmt.Sprint(status)) {
					t.Fatalf("unsafe or missing status error: %v", err)
				}
				var statusErr *apiStatusError
				if !errors.As(err, &statusErr) || statusErr.code != "" || statusErr.errorType != "" {
					t.Fatalf("unknown provider labels were retained: %v", err)
				}
				wantFatal := status == 401 || status == 403 || status == 404
				if IsFatalAPIError(err) != wantFatal {
					t.Fatalf("fatal=%t, want %t", IsFatalAPIError(err), wantFatal)
				}
			})
		}
	}
}

func TestAPIStatusErrorLabelAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name, body, code, errorType string
	}{
		{"known", `{"error":{"message":"private-prompt","code":"context_length_exceeded","type":"invalid_request_error"}}`, "context_length_exceeded", "invalid_request_error"},
		{"quota", `{"error":{"code":"insufficient_quota","type":"rate_limit_error"}}`, "insufficient_quota", "rate_limit_error"},
		{"unknown", `{"error":{"code":"opaque_api_secret","type":"provider_response_secret"}}`, "", ""},
		{"mixed", `{"error":{"code":"opaque_api_secret","type":"authentication_error"}}`, "", "authentication_error"},
		{"numeric code", `{"error":{"code":123,"type":"server_error"}}`, "", "server_error"},
		{"malformed", `{"error":{"code":"invalid_api_key"}`, "", ""},
		{"oversized", `{"error":{"code":"invalid_api_key","message":"` + strings.Repeat("x", 64<<10) + `"}}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := readAPIStatusError(http.StatusBadRequest, strings.NewReader(tc.body))
			if err.code != tc.code || err.errorType != tc.errorType || IsFatalAPIError(err) {
				t.Fatalf("provider labels or status classification changed: %+v", err)
			}
			for _, secret := range []string{"private-prompt", "opaque_api_secret", "provider_response_secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %v", secret, err)
				}
			}
			for _, label := range []string{tc.code, tc.errorType} {
				if label != "" && !strings.Contains(err.Error(), label) {
					t.Fatalf("known label missing from error: %v", err)
				}
			}
		})
	}
}

func TestSafeRequestErrorsPreserveRetrySemantics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		fatal bool
	}{
		{"canceled", context.Canceled, true},
		{"deadline", context.DeadlineExceeded, false},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, true},
		{"missing host", &net.DNSError{Err: "no such host", Name: "private-host", IsNotFound: true}, true},
		{"timeout", &net.DNSError{Err: "i/o timeout", Name: "private-host", IsTimeout: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := &url.Error{Op: "Post", URL: "https://user:password@example.com/private-key?api_key=secret", Err: tc.cause}
			err := safeFailure("API request", wrapped)
			if !errors.Is(err, tc.cause) || IsFatalAPIError(err) != tc.fatal {
				t.Fatalf("classification changed: %v", err)
			}
			for _, secret := range []string{"password", "private-key", "secret", "private-host"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("transport error leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func TestSafeEndpointAndFinishReason(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://user:password@example.com/api/coding/paas/v4/chat/completions?api_key=secret#private", "https://example.com/api/coding/paas/v4/chat/completions"},
		{"https://example.com/tenant-secret/v1/models/private-model", "https://example.com/[redacted]/v1/models/[redacted]"},
		{"https://example.com/%73ecret/v123456/chat/completions", "https://example.com/[redacted]/[redacted]/chat/completions"},
		{"http://127.0.0.1:1234/v1/chat/completions", "http://127.0.0.1:1234/v1/chat/completions"},
		{"https://%zz", "invalid"},
	} {
		if got := safeEndpoint(tc.raw); got != tc.want {
			t.Errorf("safe endpoint=%q, want %q", got, tc.want)
		}
	}
	if safeFinishReason("private-provider-response") != "other" || safeFinishReason("length") != "length" {
		t.Fatal("finish reason allowlist changed")
	}
}

func TestRequestDiagnosticsTrackFallbackAndOptionalOutput(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		t.Run(fmt.Sprintf("detailed=%t", detailed), func(t *testing.T) {
			mode := "0"
			if detailed {
				mode = "1"
			}
			t.Setenv("STORY_DIAGNOSTICS", mode)
			dir := t.TempDir()
			devlog.Init(dir, "1.0.0")
			t.Cleanup(func() { devlog.Close() })
			const input = "private-input-prompt"
			const output = "private-model-output"
			const apiKey = "unique-api-secret"
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request ChatRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Stream {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"error":{"message":%q,"code":"context_length_exceeded","type":"invalid_request_error"}}`, input+apiKey)
					return
				}
				fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`, output)
			}))
			defer server.Close()
			cfg := &config.APIConfig{BaseURL: server.URL + "/private-tenant", Model: "test-model", APIKey: apiKey, MaxTokens: 123}
			ctx := devlog.WithTask(context.Background(), "task-real-42")
			got, err := CallAPIMessages(ctx, cfg, []Message{{Role: "user", Content: input}})
			if err != nil || got != output || calls != 2 {
				t.Fatalf("fallback content=%q calls=%d error=%v", got, calls, err)
			}
			body, err := os.ReadFile(filepath.Join(dir, "app.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{input, output, apiKey, "private-tenant"} {
				if strings.Contains(string(body), secret) {
					t.Fatalf("metadata log leaked %q", secret)
				}
			}
			starts := map[string]bool{}
			ends := map[string]bool{}
			successID := ""
			for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					t.Fatal(err)
				}
				if entry["event"] != "llm_request_start" && entry["event"] != "llm_request_end" {
					continue
				}
				id, _ := entry["request_id"].(string)
				if id == "" || entry["task_id"] != "task-real-42" || entry["model"] != cfg.Model ||
					entry["endpoint"] != server.URL+"/[redacted]/v1/chat/completions" ||
					entry["input_budget"] != float64(PromptInputBudget(cfg)) || entry["max_tokens"] != float64(cfg.MaxTokens) {
					t.Fatalf("missing request correlation or metadata: %v", entry)
				}
				stream, ok := entry["stream"].(bool)
				if !ok {
					t.Fatalf("missing stream mode: %v", entry)
				}
				if entry["event"] == "llm_request_start" {
					if _, exists := starts[id]; exists {
						t.Fatalf("request ID reused: %s", id)
					}
					starts[id] = stream
					continue
				}
				ends[id] = stream
				if duration, ok := entry["duration_ms"].(float64); !ok || duration < 0 {
					t.Fatalf("missing request duration: %v", entry)
				}
				if stream {
					if entry["status_code"] != float64(400) || entry["result"] != "error" || entry["fatal"] != false ||
						entry["error_code"] != "context_length_exceeded" || entry["error_type"] != "invalid_request_error" {
						t.Fatalf("wrong stream failure: %v", entry)
					}
				} else {
					successID = id
					if entry["status_code"] != float64(200) || entry["result"] != "success" || entry["finish_reason"] != "stop" ||
						entry["prompt_tokens"] != float64(12) || entry["completion_tokens"] != float64(3) || entry["total_tokens"] != float64(15) {
						t.Fatalf("wrong completion metadata: %v", entry)
					}
				}
			}
			if len(starts) != calls || len(ends) != calls || successID == "" {
				t.Fatalf("request event counts: starts=%d ends=%d calls=%d", len(starts), len(ends), calls)
			}
			for id, stream := range starts {
				if endStream, ok := ends[id]; !ok || endStream != stream {
					t.Fatalf("unpaired request ID %s", id)
				}
			}
			payload, err := os.ReadFile(filepath.Join(dir, "diagnostics.log"))
			if !detailed {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("default diagnostics created payload file: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(payload), output) || !strings.Contains(string(payload), successID) || !strings.Contains(string(payload), "task-real-42") ||
				strings.Contains(string(payload), input) || strings.Contains(string(payload), apiKey) {
				t.Fatalf("optional output lacks correlation or leaks request data: %s", payload)
			}
		})
	}
}
