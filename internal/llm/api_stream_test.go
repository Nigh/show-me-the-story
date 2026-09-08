package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"showmethestory/internal/config"
)

func TestStreamIntegrity(t *testing.T) {
	chunk := func(content, reason string) string {
		return fmt.Sprintf("data:{\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":%q}]}\r\n\r\n", content, reason)
	}
	long := strings.Repeat("章", 30000)
	for _, tt := range []struct {
		name, wire, want, reason, errText string
		shortBody                         bool
	}{
		{name: "done", wire: chunk("ok", "") + "data: [DONE]", want: "ok"},
		{name: "finish", wire: chunk("ok", "stop"), want: "ok", reason: "stop"},
		{name: "large event", wire: chunk(long, "stop"), want: long, reason: "stop"},
		{name: "early EOF", wire: chunk("partial", ""), want: "partial", errText: "without finish_reason"},
		{name: "broken JSON", wire: chunk("partial", "") + "data: {broken}\n\ndata: [DONE]\n", want: "partial", errText: "invalid SSE JSON"},
		{name: "read error", wire: chunk("partial", ""), want: "partial", shortBody: true, errText: "unexpected EOF"},
		{name: "length", wire: chunk("partial", "length"), want: "partial", reason: "length"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req ChatRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.MaxTokens != 32768 || !req.Stream {
					t.Errorf("request budget/stream changed: %+v", req)
				}
				if tt.shortBody {
					w.Header().Set("Content-Length", fmt.Sprint(len(tt.wire)+100))
				}
				fmt.Fprint(w, tt.wire)
			}))
			defer srv.Close()
			cfg := &config.APIConfig{BaseURL: srv.URL, MaxTokens: 32768}
			var chunks strings.Builder
			got, err := CallAPIStreamMessages(context.Background(), cfg, nil, func(s string) { chunks.WriteString(s) })
			if got.Content != tt.want || chunks.String() != tt.want || got.FinishReason != tt.reason {
				t.Fatalf("content or finish reason mismatch: content bytes=%d reason=%q", len(got.Content), got.FinishReason)
			}
			if tt.errText == "" && err != nil || tt.errText != "" && (err == nil || !strings.Contains(err.Error(), tt.errText)) {
				t.Fatalf("error=%v, want %q", err, tt.errText)
			}
		})
	}
}

func TestBufferedCompletionErrors(t *testing.T) {
	for _, mode := range []string{"partial", "length", "fallback", "fallback length", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req ChatRequest
				json.NewDecoder(r.Body).Decode(&req)
				if req.Stream {
					if strings.HasPrefix(mode, "fallback") {
						http.Error(w, "stream unsupported", http.StatusBadRequest)
						return
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
					if mode == "length" {
						fmt.Fprint(w, "data: {\"choices\":[{\"finish_reason\":\"length\"}]}\n\n")
					}
					if mode == "timeout" {
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}
					return
				}
				reason := "stop"
				if mode == "fallback length" {
					reason = "length"
				}
				fmt.Fprintf(w, "{\"choices\":[{\"message\":{\"content\":\"complete\"},\"finish_reason\":%q}]}", reason)
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := CallAPIMessages(ctx, &config.APIConfig{BaseURL: srv.URL, MaxTokens: 32768}, nil)
			if (mode == "fallback") != (err == nil) {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(mode, "length") && (!IsFatalAPIError(err) || !strings.Contains(err.Error(), "finish_reason=length")) {
				t.Fatalf("missing non-retryable limit diagnostic: %v", err)
			}
			wantCalls := 1
			if strings.HasPrefix(mode, "fallback") {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d, want %d", calls, wantCalls)
			}
		})
	}
}
