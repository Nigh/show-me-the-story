package i18n

import (
	"strings"
	"testing"
)

func TestAgentStepMessages(t *testing.T) {
	for _, tc := range []struct {
		key  string
		args []any
		want string
	}{
		{"log.agent_step_messages", []any{1, 10, 3, []string{"system", "user"}}, "[Agent] Step 1/10: 3 messages: [system user]"},
		{"log.agent_step_api_failed", []any{1, "offline"}, "[Agent] Step 1: API call failed: offline"},
		{"log.agent_step_response", []any{1, 42, "stop"}, "[Agent] Step 1: API response 42 characters (finish_reason=stop)"},
		{"log.agent_step_final", []any{1, "原文"}, "[Agent] Step 1: No tool call detected; returning final reply. Preview: 原文"},
		{"log.agent_step_tool", []any{1, "search_project"}, "[Agent] Step 1: Tool call detected → search_project"},
		{"log.agent_step_tool_done", []any{1, "search_project", "原文"}, "[Agent] Step 1: Tool search_project completed. Result: 原文"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if got := T(LangEN, tc.key, tc.args...); got != tc.want {
				t.Fatalf("English message = %q, want %q", got, tc.want)
			}
			if got := T(LangZH, tc.key, tc.args...); !strings.Contains(got, "步骤 1") || strings.Contains(got, "%!") {
				t.Fatalf("invalid Chinese message: %q", got)
			}
		})
	}
}

func TestOutlineTitlePromptConstraints(t *testing.T) {
	for _, key := range []string{"outline_editor_json", "outline_editor_locked_json", "outline_editor_brief_json"} {
		for _, lang := range []string{LangZH, LangEN} {
			prompt := SystemPromptFor(lang, key)
			required := []string{"title", "章号", "引号"}
			if lang == LangEN {
				required = []string{"title", "without chapter numbers", "surrounding quotation marks"}
			}
			for _, part := range required {
				if !strings.Contains(prompt, part) {
					t.Errorf("%s/%s missing %q", key, lang, part)
				}
			}
		}
	}
}
