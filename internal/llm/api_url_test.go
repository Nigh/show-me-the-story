package llm

import "testing"

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
