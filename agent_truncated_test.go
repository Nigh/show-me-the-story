package main

import "testing"

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
	got := extractJSON(content)
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

func TestAgentEffectiveMaxTokens(t *testing.T) {
	if agentEffectiveMaxTokens(&APIConfig{MaxTokens: 0}) != 8192 {
		t.Fatal("Agent max_tokens 下限应为 8192")
	}
	if agentEffectiveMaxTokens(&APIConfig{MaxTokens: 16000}) != 16000 {
		t.Fatal("应使用用户配置的 max_tokens")
	}
}
