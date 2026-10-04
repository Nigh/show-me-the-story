package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"showmethestory/internal/config"
	"showmethestory/internal/llm"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAgentPartialStreamDoesNotAppendFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	}))
	defer srv.Close()
	var output strings.Builder
	_, err := callAgentAPI(context.Background(), &config.APIConfig{BaseURL: srv.URL}, nil, func(s string) { output.WriteString(s) })
	if err == nil || calls != 1 || output.String() != "partial" {
		t.Fatalf("err=%v calls=%d output=%q", err, calls, output.String())
	}
}

func TestParseToolCallTruncatedJSONNoRepair(t *testing.T) {
	// 流式截断：缺 </tool_call> 且 JSON 不完整 — 不修复，parseToolCall 应返回 nil
	truncated := `明白，开始修订第3章。 <tool_call> {"name":"revise_chapter","arguments":{"num":3,"feedback":"完全重写第3章，解决逻辑硬伤：\n\n1.【删掉借鞋借袜】男女鞋码差4-5码，借鞋穿不上；正常女生不会把袜子给陌生男性。这两个情节必须彻底删除。\n\n2.【新借口】周凯伪装成物业检修工。他提前在网上买了件类似物业维修的蓝色马甲，手里`

	tc := parseToolCall(truncated)
	if tc != nil {
		t.Fatalf("parseToolCall 不应修复截断 JSON，实际解析到工具: %s", tc.Name)
	}
}

func TestParseToolCallCompleteTag(t *testing.T) {
	complete := `<tool_call>{"name":"search_project","arguments":{"query":"人物"}}</tool_call>`
	tc := parseToolCall(complete)
	if tc == nil {
		t.Fatal("完整工具调用解析失败")
	}
	if tc.Name != "search_project" {
		t.Fatalf("工具名应为 search_project，实际: %s", tc.Name)
	}
}

func TestParseToolCallTextBeforeTag(t *testing.T) {
	content := `好的，我来修改。 <tool_call>{"name":"revise_chapter","arguments":{"num":1,"feedback":"修改第一章"}}</tool_call>`
	tc := parseToolCall(content)
	if tc == nil {
		t.Fatal("标签前有文字时解析失败")
	}
	if tc.Name != "revise_chapter" {
		t.Fatalf("工具名应为 revise_chapter，实际: %s", tc.Name)
	}
}

func TestExtractJSONStringAware(t *testing.T) {
	content := `{"name":"x","arguments":{"feedback":"他说：} 这个符号"}}`
	got := llm.ExtractJSON(content)
	tc := parseToolCallFromJSON(got)
	if tc == nil {
		t.Fatalf("字符串感知提取失败，got: %q", got)
	}
	if tc.Name != "x" {
		t.Fatalf("工具名应为 x，实际: %s", tc.Name)
	}
}

func TestHasUnclosedToolCall(t *testing.T) {
	if !hasUnclosedToolCall(`<tool_call>{"name":"x"}`) {
		t.Fatal("应识别未闭合 tool_call")
	}
	if hasUnclosedToolCall(`<tool_call>{"name":"x"}</tool_call>`) {
		t.Fatal("完整 tool_call 不应判为未闭合")
	}
	if hasUnclosedToolCall(`no tool here`) {
		t.Fatal("无 tool_call 时应为 false")
	}
}

func TestIsAgentOutputTruncated(t *testing.T) {
	truncated := `前言 <tool_call> {"name":"revise_chapter","arguments":{"feedback":"hello`
	if !isAgentOutputTruncated("length", truncated, nil) {
		t.Fatal("finish_reason=length + 未闭合 tool_call + 解析失败 应判为截断错误")
	}
	if isAgentOutputTruncated("stop", truncated, nil) {
		t.Fatal("finish_reason=stop 不应判为 token 截断")
	}
	complete := `<tool_call>{"name":"x","arguments":{}}</tool_call>`
	tc := parseToolCall(complete)
	if isAgentOutputTruncated("length", complete, tc) {
		t.Fatal("完整 tool_call 即使 finish_reason=length 也不应报错（由 provider 误报时保守通过）")
	}
}

func TestIsFailedToolCallAttempt(t *testing.T) {
	unclosed := `<tool_call> {"name":"update_project_config","arguments":{"title":"颂歌`
	if !isFailedToolCallAttempt(unclosed, parseToolCall(unclosed)) {
		t.Fatal("未闭合 tool_call 且解析失败应视为失败尝试")
	}
	badClosed := `<tool_call>{"name":</tool_call>`
	if !isFailedToolCallAttempt(badClosed, parseToolCall(badClosed)) {
		t.Fatal("闭合但非法 JSON 应视为失败尝试")
	}
	ok := `<tool_call>{"name":"search_project","arguments":{"query":"x"}}</tool_call>`
	if isFailedToolCallAttempt(ok, parseToolCall(ok)) {
		t.Fatal("合法 tool_call 不应视为失败尝试")
	}
	if isFailedToolCallAttempt("直接回复用户，无需工具", nil) {
		t.Fatal("普通最终回复不应视为失败尝试")
	}
}

func TestToolCallParseRetryFeedback(t *testing.T) {
	ctx := &AgentContext{Config: &config.Config{Language: "zh"}, APICfg: &config.APIConfig{MaxTokens: 8192}}
	fb := toolCallParseRetryFeedback(ctx, "stop", `<tool_call>{"name":"x"`)
	if !strings.Contains(fb, "请重试一次") {
		t.Fatalf("反馈应要求重试，实际: %s", fb)
	}
	if !strings.Contains(fb, "truncated_or_unclosed") {
		t.Fatalf("未闭合时应标记 truncated_or_unclosed，实际: %s", fb)
	}
}

func TestAgentEffectiveMaxTokens(t *testing.T) {
	if agentEffectiveMaxTokens(&config.APIConfig{MaxTokens: 0}) != 8192 {
		t.Fatal("Agent max_tokens 下限应为 8192")
	}
	if agentEffectiveMaxTokens(&config.APIConfig{MaxTokens: 16000}) != 16000 {
		t.Fatal("应使用用户配置的 max_tokens")
	}
}

func TestBuildAgentMessagesBoundsHistory(t *testing.T) {
	ctx := &AgentContext{
		APICfg: &config.APIConfig{ContextBudgetTokens: 30000, MaxTokens: 8192},
		Config: &config.Config{Language: "en"},
	}
	oldResult := strings.Repeat("old-tool-result ", 5000)
	history := []AgentStep{
		{Role: "user", Content: "earlier request"},
		{Role: "assistant", Content: "<think>private chain</think>visible reply"},
		{Role: "user", Content: "current request"},
		{Role: "assistant", ToolCall: &ToolCall{Name: "read_chapter"}},
		{Role: "tool", ToolResult: oldResult},
		{Role: "assistant", ToolCall: &ToolCall{Name: "read_chapter"}},
		{Role: "tool", ToolResult: "recent tool result one"},
		{Role: "assistant", ToolCall: &ToolCall{Name: "read_chapter"}},
		{Role: "tool", ToolResult: "recent tool result two"},
	}

	messages := buildAgentMessages(ctx, "system", "current request", history, "[Tool result]", nil)
	prompt := strings.Join(func() []string {
		parts := make([]string, len(messages))
		for i, message := range messages {
			parts[i] = message.Content
		}
		return parts
	}(), "\n")

	if strings.Contains(prompt, oldResult) {
		t.Fatal("old tool result was retained")
	}
	if !strings.Contains(prompt, "Earlier tool result omitted") {
		t.Fatal("old tool result was not replaced with an omission record")
	}
	if strings.Contains(prompt, "<think>") || !strings.Contains(prompt, "visible reply") {
		t.Fatalf("assistant reasoning was not stripped: %q", prompt)
	}
	assertAgentPromptWithinBudget(t, ctx, messages)
}

func TestBuildAgentMessagesTruncatesLatestToolResult(t *testing.T) {
	ctx := &AgentContext{
		APICfg: &config.APIConfig{ContextBudgetTokens: 24000, MaxTokens: 8192},
		Config: &config.Config{Language: "en"},
	}
	latestResult := strings.Repeat("latest-tool-result ", 50000)
	history := []AgentStep{
		{Role: "assistant", ToolCall: &ToolCall{Name: "read_chapter"}},
		{Role: "tool", ToolResult: latestResult},
	}

	messages := buildAgentMessages(ctx, "system", "current request", history, "[Tool result]", nil)
	prompt := strings.Join(func() []string {
		parts := make([]string, len(messages))
		for i, message := range messages {
			parts[i] = message.Content
		}
		return parts
	}(), "\n")

	if strings.Contains(prompt, latestResult) {
		t.Fatal("latest oversized tool result was not truncated")
	}
	if !strings.Contains(prompt, "[Tool result truncated.") {
		t.Fatal("truncated tool result lacks its marker")
	}
	assertAgentPromptWithinBudget(t, ctx, messages)
}

func TestBuildAgentMessagesBoundsRetryTail(t *testing.T) {
	ctx := &AgentContext{
		APICfg: &config.APIConfig{ContextBudgetTokens: 24000, MaxTokens: 8192},
		Config: &config.Config{Language: "en"},
	}
	tail := []llm.Message{
		{Role: "assistant", Content: strings.Repeat("broken tool call ", 50000)},
		{Role: "user", Content: "retry the malformed tool call"},
	}

	messages := buildAgentMessages(ctx, "system", "current request", nil, "[Tool result]", tail)
	prompt := strings.Join(func() []string {
		parts := make([]string, len(messages))
		for i, message := range messages {
			parts[i] = message.Content
		}
		return parts
	}(), "\n")

	if !strings.Contains(prompt, "retry the malformed tool call") {
		t.Fatal("retry feedback was dropped")
	}
	if !strings.Contains(prompt, "[Previous malformed response truncated.]") {
		t.Fatal("malformed response lacks its truncation marker")
	}
	assertAgentPromptWithinBudget(t, ctx, messages)
}

func TestBuildAgentMessagesKeepsCurrentTurnOrder(t *testing.T) {
	ctx := &AgentContext{
		APICfg: &config.APIConfig{ContextBudgetTokens: 24000, MaxTokens: 8192},
		Config: &config.Config{Language: "en"},
	}
	tests := []struct {
		name    string
		history []AgentStep
		tail    []llm.Message
		want    []string
	}{
		{
			name:    "first request",
			history: []AgentStep{{Role: "user", Content: "create Mira"}},
			want:    []string{"system", "create Mira"},
		},
		{
			name: "consecutive tools",
			history: []AgentStep{
				{Role: "user", Content: "older request"},
				{Role: "assistant", Content: "older reply"},
				{Role: "user", Content: "create Mira"},
				{Role: "assistant", ToolCall: &ToolCall{Name: "create_character"}},
				{Role: "tool", ToolResult: "Mira created"},
				{Role: "assistant", ToolCall: &ToolCall{Name: "read_character"}},
				{Role: "tool", ToolResult: "Mira exists"},
			},
			want: []string{"system", "older request", "older reply", "create Mira", "<tool_call>", "Mira created", "<tool_call>", "Mira exists"},
		},
		{
			name: "parse retry",
			history: []AgentStep{
				{Role: "user", Content: "create Mira"},
				{Role: "assistant", ToolCall: &ToolCall{Name: "create_character"}},
				{Role: "tool", ToolResult: "Mira created"},
			},
			tail: []llm.Message{{Role: "assistant", Content: "malformed call"}, {Role: "user", Content: "retry feedback"}},
			want: []string{"system", "create Mira", "<tool_call>", "Mira created", "malformed call", "retry feedback"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := buildAgentMessages(ctx, "system", "create Mira", tt.history, "[Tool result]", tt.tail)
			if len(messages) != len(tt.want) {
				t.Fatalf("messages=%v; want %d messages", messages, len(tt.want))
			}
			for i, want := range tt.want {
				if !strings.Contains(messages[i].Content, want) {
					t.Fatalf("message %d = %q; want %q", i, messages[i].Content, want)
				}
			}
			assertAgentPromptWithinBudget(t, ctx, messages)
		})
	}
}

func TestBuildAgentMessagesBoundsCurrentToolResultAtPreflight(t *testing.T) {
	ctx := &AgentContext{
		APICfg: &config.APIConfig{ContextBudgetTokens: 24000, MaxTokens: 8192},
		Config: &config.Config{Language: "en"},
	}
	history := []AgentStep{
		{Role: "user", Content: "u"},
		{Role: "assistant", ToolCall: &ToolCall{Name: "read_chapter"}},
		{Role: "tool", ToolResult: strings.Repeat("x", 50000)},
	}
	messages := buildAgentMessages(ctx, "s", "u", history, "[Tool result]", nil)
	if len(messages) != 4 || messages[1].Content != "u" || !strings.Contains(messages[3].Content, "[Tool result truncated.") {
		t.Fatalf("current request or truncated tool result missing: %v", messages)
	}
	assertAgentPromptWithinBudget(t, ctx, messages)
}

func assertAgentPromptWithinBudget(t *testing.T, ctx *AgentContext, messages []llm.Message) {
	t.Helper()
	runes := 0
	for _, message := range messages {
		runes += utf8.RuneCountInString(message.Content)
	}
	if got, budget := llm.EstimateTokensFromRunes(runes), agentPromptInputBudget(ctx.APICfg); got > budget {
		t.Fatalf("LLM preflight tokens=%d exceed budget=%d", got, budget)
	}
}

func TestTruncateAgentContentStaysWithinBudget(t *testing.T) {
	for budget := 1; budget <= 30; budget++ {
		got := truncateAgentContent(strings.Repeat("x", 100), budget, ".")
		if tokens := llm.EstimateTokensFromRunes(len(got)); tokens > budget {
			t.Fatalf("budget=%d: truncated content uses %d tokens: %q", budget, tokens, got)
		}
	}
}
