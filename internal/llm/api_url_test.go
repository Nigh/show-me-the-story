package llm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"showmethestory/internal/config"
)

func TestResolveChatCompletionsURL(t *testing.T) {
	tests := []struct {
		base   string
		strict bool
		want   string
	}{
		{"https://api.z.ai/api/paas/v4", false, "https://api.z.ai/api/paas/v4/chat/completions"},
		{"https://api.z.ai/api/coding/paas/v4", true, "https://api.z.ai/api/coding/paas/v4/chat/completions"},
		{"https://api.deepseek.com", false, "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com", true, "https://api.deepseek.com/chat/completions"},
		{"https://api.openai.com/v1", false, "https://api.openai.com/v1/chat/completions"},
		{"https://api.z.ai/api/paas/v4/chat/completions", false, "https://api.z.ai/api/paas/v4/chat/completions"},
		{"  https://api.example.com/v1/  ", false, "https://api.example.com/v1/chat/completions"},
		{"", false, ""},
	}
	for _, tc := range tests {
		got := resolveChatCompletionsURL(tc.base, tc.strict)
		if got != tc.want {
			t.Errorf("resolveChatCompletionsURL(%q, %v) = %q, want %q", tc.base, tc.strict, got, tc.want)
		}
	}
}

func TestEnsureContextBudgetClampsOnlyDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models/test" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"context_window":128000}`)
	}))
	defer server.Close()

	large := &config.APIConfig{BaseURL: server.URL, Model: "test", ContextBudgetTokens: 300000}
	EnsureContextBudget(large)
	if large.ContextBudgetTokens != 128000 {
		t.Fatalf("large budget = %d", large.ContextBudgetTokens)
	}
	small := &config.APIConfig{BaseURL: server.URL, Model: "test", ContextBudgetTokens: 64000}
	EnsureContextBudget(small)
	if small.ContextBudgetTokens != 64000 {
		t.Fatalf("small budget = %d", small.ContextBudgetTokens)
	}
}
