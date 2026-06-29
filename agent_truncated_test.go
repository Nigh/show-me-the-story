package main

import "testing"

func TestParseToolCallTruncatedJSON(t *testing.T) {
	// 模拟 Bug 场景：AI 输出 <tool_call> 但响应被截断，
	// 缺少 </tool_call> 和 JSON 结尾的 }}
	// 注意：真实 AI 输出中 JSON 字符串内的换行是 \n 转义序列（两字符），非字面换行
	truncated := `明白，开始修订第3章。 <tool_call> {"name":"revise_chapter","arguments":{"num":3,"feedback":"完全重写第3章，解决逻辑硬伤：\n\n1.【删掉借鞋借袜】男女鞋码差4-5码，借鞋穿不上；正常女生不会把袜子给陌生男性。这两个情节必须彻底删除。\n\n2.【新借口】周凯伪装成物业检修工。他提前在网上买了件类似物业维修的蓝色马甲，手里`

	tc := parseToolCall(truncated)
	if tc == nil {
		t.Fatal("parseToolCall 返回 nil，未能识别截断的工具调用")
	}
	if tc.Name != "revise_chapter" {
		t.Fatalf("工具名应为 revise_chapter，实际: %s", tc.Name)
	}
	t.Logf("成功识别截断的工具调用: %s", tc.Name)
}

func TestParseToolCallCompleteTag(t *testing.T) {
	// 完整的 <tool_call>...</tool_call> 应正常解析
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
	// 标签前有解释文字（违反 prompt 但 AI 偶尔会这么做）
	content := `好的，我来修改。 <tool_call>{"name":"revise_chapter","arguments":{"num":1,"feedback":"修改第一章"}}</tool_call>`
	tc := parseToolCall(content)
	if tc == nil {
		t.Fatal("标签前有文字时解析失败")
	}
	if tc.Name != "revise_chapter" {
		t.Fatalf("工具名应为 revise_chapter，实际: %s", tc.Name)
	}
}

func TestRepairTruncatedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		ok    bool
	}{
		{"完整JSON", `{"name":"x","arguments":{"num":1}}`, true},
		{"缺尾部}}", `{"name":"x","arguments":{"num":1,"feedback":"hello`, true},
		{"缺尾部}字符串未闭", `{"name":"revise_chapter","arguments":{"num":3,"feedback":"test`, true},
		{"空字符串", "", false},
		{"非JSON", "hello world", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := repairTruncatedJSON(tt.input)
			if tt.ok {
				// 验证修复后能被 parseToolCallFromJSON 解析
				tc := parseToolCallFromJSON(repaired)
				if tc == nil {
					t.Fatalf("修复后仍无法解析: input=%q repaired=%q", tt.input, repaired)
				}
				t.Logf("修复成功: %s -> %s (工具: %s)", tt.name, repaired, tc.Name)
			}
		})
	}
}

func TestExtractJSONStringAware(t *testing.T) {
	// JSON 字符串内包含 } 字符，extractJSON 不应提前截断
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
