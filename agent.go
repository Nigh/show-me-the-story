package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type Tool struct {
	Name        string
	Description string
	Parameters  string
	Execute     func(args json.RawMessage, ctx *AgentContext) (string, error)
}

type AgentContext struct {
	APICfg       *APIConfig
	Settings     *ProjectSettings
	SettingsPath string
	State        *Progress
	Config       *Config
	Skills       []Skill
	Logger       *LogBroadcaster
	ContextPage  string
	ProgressPath string
	CfgPath      string
	SessionsDir  string
	ProjectDir   string
	StartAsync   func(taskName string, fn func(goCtx context.Context))
}

type AgentStep struct {
	Role       string
	Content    string
	ToolCall   *ToolCall
	ToolResult string
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func RunAgentLoop(goCtx context.Context, ctx *AgentContext, userMessage string, history []AgentStep, maxSteps int) (string, []AgentStep, error) {
	tools := getBuiltinTools()
	toolDesc := buildToolDescriptions(tools)

	systemPrompt := buildAgentSystemPrompt(ctx, toolDesc)

	var messages []Message
	messages = append(messages, Message{Role: "system", Content: systemPrompt})

	toolResultLabel := "[工具结果]"
	if NormalizeLanguage(ctx.Config.Language) == LangEN {
		toolResultLabel = "[Tool result]"
	}

	for _, step := range history {
		if step.Role == "assistant" {
			if step.ToolCall != nil {
				tcJSON, _ := json.Marshal(step.ToolCall)
				messages = append(messages, Message{Role: "assistant", Content: fmt.Sprintf("<tool_call>\n%s\n</tool_call>", string(tcJSON))})
			} else {
				messages = append(messages, Message{Role: "assistant", Content: step.Content})
			}
		} else if step.Role == "tool" {
			messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("%s\n%s", toolResultLabel, step.ToolResult)})
		}
	}

	messages = append(messages, Message{Role: "user", Content: userMessage})

	for step := 0; step < maxSteps; step++ {
		if goCtx.Err() != nil {
			return "", history, fmt.Errorf("任务已取消")
		}

		fullResp := ""
		err := callAgentAPI(goCtx, ctx.APICfg, messages, func(chunk string) {
			fullResp += chunk
		})
		if err != nil {
			return "", history, fmt.Errorf("Agent API 调用失败: %w", err)
		}

		toolCall := parseToolCall(fullResp)

		if toolCall == nil {
			history = append(history, AgentStep{Role: "assistant", Content: fullResp})
			return fullResp, history, nil
		}

		history = append(history, AgentStep{Role: "assistant", Content: fullResp, ToolCall: toolCall})

		if ctx.Logger != nil {
			ctx.Logger.ToolCallStart("", toolCall.Name, string(toolCall.Arguments))
		}

		result := executeTool(toolCall, tools, ctx)

		history = append(history, AgentStep{Role: "tool", ToolResult: result})

		if ctx.Logger != nil {
			ctx.Logger.ToolCallEnd("", toolCall.Name, truncate(result, 200))
		}

		messages = append(messages, Message{Role: "assistant", Content: fmt.Sprintf("<tool_call>\n%s\n</tool_call>", func() string {
			tcJSON, _ := json.Marshal(toolCall)
			return string(tcJSON)
		}())})
		messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("%s\n%s", toolResultLabel, result)})
	}

	if NormalizeLanguage(ctx.Config.Language) == LangEN {
		return "Reached the maximum tool-call step limit.", history, nil
	}
	return "已达到最大工具调用步骤限制。", history, nil
}

func callAgentAPI(ctx context.Context, apiCfg *APIConfig, messages []Message, onChunk func(string)) error {
	// 以真实的多轮消息结构调用，保留对话角色信息
	_, err := CallAPIStreamMessages(ctx, apiCfg, messages, onChunk)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		result, err2 := CallAPIMessages(ctx, apiCfg, messages)
		if err2 != nil {
			return err
		}
		if onChunk != nil {
			onChunk(result)
		}
	}
	return nil
}

func buildAgentSystemPrompt(ctx *AgentContext, toolDesc string) string {
	if NormalizeLanguage(ctx.Config.Language) == LangEN {
		return buildAgentSystemPromptEN(ctx, toolDesc)
	}
	return buildAgentSystemPromptZH(ctx, toolDesc)
}

func buildAgentSystemPromptZH(ctx *AgentContext, toolDesc string) string {
	var sb strings.Builder
	sb.WriteString("你是一个小说创作助手，全权负责管理小说项目的一切操作，包括：生成/修订/确认大纲、生成/修订/确认章节、管理角色/世界观/组织/关系/伏笔、技能管理、项目配置等。\n\n")

	sb.WriteString("## 项目信息\n")
	if ctx.State.Title != "" {
		sb.WriteString(fmt.Sprintf("小说标题: 《%s》\n", ctx.State.Title))
	}
	sb.WriteString(fmt.Sprintf("当前阶段: %s\n", ctx.State.Phase))
	sb.WriteString(fmt.Sprintf("章节数: %d\n", len(ctx.State.Chapters)))

	if ctx.Settings != nil {
		sb.WriteString(fmt.Sprintf("角色数: %d\n", len(ctx.Settings.Characters)))
		sb.WriteString(fmt.Sprintf("世界观条目: %d\n", len(ctx.Settings.Worldview)))
		sb.WriteString(fmt.Sprintf("组织数: %d\n", len(ctx.Settings.Organizations)))
	}

	if ctx.ContextPage != "" {
		pageNames := map[string]string{
			"config":    "配置",
			"outline":   "大纲",
			"writing":   "写作",
			"relations": "图谱",
			"skills":    "技能",
		}
		if name, ok := pageNames[ctx.ContextPage]; ok {
			sb.WriteString(fmt.Sprintf("\n用户当前正在查看「%s」页面。\n", name))
		}
	}

	sb.WriteString("\n")

	enabledSkills := GetEnabledSkills(ctx.Skills, ctx.Config.SkillConfig)
	if len(enabledSkills) > 0 {
		sb.WriteString("## 已启用技能\n")
		sb.WriteString(FormatSkillsContent(enabledSkills))
		sb.WriteString("\n")
	}

	sb.WriteString("## 可用工具\n")
	sb.WriteString(toolDesc)
	sb.WriteString("\n\n")

	sb.WriteString("## 工具调用格式\n")
	sb.WriteString("当需要调用工具时，严格使用以下格式。注意：必须是合法的JSON，不要用XML标签包裹：\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString(`{"name": "工具名称", "arguments": {"参数名": "参数值"}}`)
	sb.WriteString("\n</tool_call>\n\n")
	sb.WriteString("正确示例：\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString(`{"name": "search_project", "arguments": {"query": "人物"}}`)
	sb.WriteString("\n</tool_call>\n\n")
	sb.WriteString("错误示例（不要这样写）：\n")
	sb.WriteString("- 不要在 tool_call 标签内使用 arguments 等XML标签\n")
	sb.WriteString("- 不要在 tool_call 标签外写工具调用JSON\n")
	sb.WriteString("- 不要输出多个 tool_call 标签\n")
	sb.WriteString("一次只能调用一个工具。等收到工具结果后再继续。\n")
	sb.WriteString("当不需要调用工具时，直接回复用户即可。\n\n")

	sb.WriteString("## 安全规则（最高优先级，违反将造成用户数据永久丢失）\n")
	sb.WriteString("1. **修改 ≠ 删除**。当用户要求「修改/调整/润色/修正某一章」时，必须且只能使用 revise_chapter 工具（通过 num 参数指定章节号）。绝对禁止通过 delete_chapter / delete_chapters_from / delete_outline / reset_progress 来实现任何形式的「修改」需求。\n")
	sb.WriteString("2. revise_chapter 支持修订任意已有内容的章节（包括已确认的早期章节），它只改动目标章节本身，不影响其他章节。修改第 6 章的细节就调用 revise_chapter(num=6, feedback=具体意见)，仅此而已。\n")
	sb.WriteString("3. 删除类工具（delete_chapter、delete_chapters_from、delete_outline、reset_progress）是不可逆的危险操作，仅当用户**明确使用「删除/清空/重置」等字眼**并指明范围时才可使用。使用前必须：先用一条纯文本回复向用户复述将被删除的确切范围（如「将删除第 6~30 章共 25 章内容及其正文文件」），等用户明确回复确认后，才在下一轮调用工具并传入 confirm=true。\n")
	sb.WriteString("4. 任何情况下都不要为了「让操作更彻底」「方便重新生成」而扩大删除范围。宁可少做，不可多删。\n")
	sb.WriteString("5. 拿不准用户意图时，先提问澄清，不要猜测着执行写操作。\n\n")

	sb.WriteString("## 工具选择指南\n")
	sb.WriteString("- 修改某章内容细节 → revise_chapter(num, feedback)\n")
	sb.WriteString("- 修改某章的大纲（未写作的 pending 章节）→ edit_chapter_outline(num, title, outline)\n")
	sb.WriteString("- 对整体大纲提修改意见 → revise_outline(feedback)（只会改动未确认章节）\n")
	sb.WriteString("- 生成下一章正文 → generate_chapter\n")
	sb.WriteString("- 已有确认章节、想追加新章节 → 不要用 generate_outline（会被拒绝），告知用户在大纲页使用「生成后续大纲」\n\n")

	sb.WriteString("## 重要规则\n")
	sb.WriteString("- 异步工具（如 generate_outline、generate_chapter 等）会立即返回「任务已启动」，任务结果通过日志推送到界面。你必须先调用工具，收到工具结果后才能告知用户任务已启动。绝对不要在没有调用工具的情况下输出「请等待」「请耐心等待」「请稍等」「正在生成」等文字——如果用户请求的操作你无法完成，直接说明原因即可。\n")
	sb.WriteString("- 当用户提交故事配置时（如「请更新以下故事配置」），使用 update_project_config 工具。\n")
	sb.WriteString("- 当用户提交写作风格或故事梗概的更新时（如「请更新写作风格:」或「请更新故事梗概:」），使用 update_project_config 工具保存对应字段。\n")
	sb.WriteString("- 当用户要求创建/修改角色、世界观等设定时，直接使用对应的工具完成操作。\n")
	sb.WriteString("- 当用户要求生成大纲、生成章节等操作时，使用对应的工具。如果是异步工具，告知用户等待。\n")
	sb.WriteString("- 在生成大纲之前，提醒用户检查配置页面中的各项设定（故事类型、写作风格、故事梗概、角色、世界观），确认无误后再进行。\n")
	sb.WriteString("- 在正式开始写作（确认大纲）之前，再次提醒用户确认所有设定，包括角色详情和世界观条目。\n")
	sb.WriteString("- 执行写操作前，优先用读工具（read_outline、read_chapter 等）确认目标存在且状态符合预期。\n")
	sb.WriteString("- 所有操作完成后，简要告知用户结果，并在末尾建议接下来可以进行的 1-2 个操作（如：检查角色设定、生成大纲、确认章节等），帮助用户推进项目。\n")

	return sb.String()
}

func buildAgentSystemPromptEN(ctx *AgentContext, toolDesc string) string {
	var sb strings.Builder
	sb.WriteString("You are a novel-writing assistant in full charge of every operation on the project: generating/revising/confirming outlines, generating/revising/confirming chapters, managing characters/worldview/organisations/relations/foreshadows, skill management, project configuration, and so on. Reply to the user in English.\n\n")

	sb.WriteString("## Project info\n")
	if ctx.State.Title != "" {
		sb.WriteString(fmt.Sprintf("Novel title: \"%s\"\n", ctx.State.Title))
	}
	sb.WriteString(fmt.Sprintf("Current phase: %s\n", ctx.State.Phase))
	sb.WriteString(fmt.Sprintf("Chapter count: %d\n", len(ctx.State.Chapters)))

	if ctx.Settings != nil {
		sb.WriteString(fmt.Sprintf("Characters: %d\n", len(ctx.Settings.Characters)))
		sb.WriteString(fmt.Sprintf("Worldview entries: %d\n", len(ctx.Settings.Worldview)))
		sb.WriteString(fmt.Sprintf("Organisations: %d\n", len(ctx.Settings.Organizations)))
	}

	if ctx.ContextPage != "" {
		pageNames := map[string]string{
			"config":    "Config",
			"outline":   "Outline",
			"writing":   "Writing",
			"relations": "Relations",
			"skills":    "Skills",
		}
		if name, ok := pageNames[ctx.ContextPage]; ok {
			sb.WriteString(fmt.Sprintf("\nThe user is currently viewing the \"%s\" page.\n", name))
		}
	}

	sb.WriteString("\n")

	enabledSkills := GetEnabledSkills(ctx.Skills, ctx.Config.SkillConfig)
	if len(enabledSkills) > 0 {
		sb.WriteString("## Enabled skills\n")
		sb.WriteString(FormatSkillsContent(enabledSkills))
		sb.WriteString("\n")
	}

	sb.WriteString("## Available tools\n")
	sb.WriteString(toolDesc)
	sb.WriteString("\n\n")

	sb.WriteString("## Tool-call format\n")
	sb.WriteString("When you need to call a tool, use this exact format. The payload must be valid JSON; do not wrap arguments in XML tags:\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString(`{"name": "tool_name", "arguments": {"arg_name": "arg_value"}}`)
	sb.WriteString("\n</tool_call>\n\n")
	sb.WriteString("Correct example:\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString(`{"name": "search_project", "arguments": {"query": "character"}}`)
	sb.WriteString("\n</tool_call>\n\n")
	sb.WriteString("Incorrect examples (do NOT do this):\n")
	sb.WriteString("- Do not use XML tags such as <arguments> inside <tool_call>\n")
	sb.WriteString("- Do not write tool-call JSON outside the <tool_call> tag\n")
	sb.WriteString("- Do not emit multiple <tool_call> tags at once\n")
	sb.WriteString("Call one tool at a time. Wait for the tool result before continuing.\n")
	sb.WriteString("If no tool is needed, just reply directly to the user.\n\n")

	sb.WriteString("## Safety rules (highest priority — violating them causes permanent user-data loss)\n")
	sb.WriteString("1. **Edit != Delete**. When the user asks to \"revise/adjust/polish/fix chapter N\", you MUST use the revise_chapter tool (pass the chapter number via the num argument). NEVER use delete_chapter / delete_chapters_from / delete_outline / reset_progress to satisfy any kind of \"edit\" request.\n")
	sb.WriteString("2. revise_chapter can revise any chapter that has content (including confirmed early chapters); it only modifies the target chapter and never touches the others. To tweak chapter 6, call revise_chapter(num=6, feedback=specific instructions). That's it.\n")
	sb.WriteString("3. Delete tools (delete_chapter, delete_chapters_from, delete_outline, reset_progress) are irreversible. Only use them when the user explicitly says \"delete/clear/reset\" and specifies the range. Before using one, first reply in plain text restating the exact range that will be deleted (e.g. \"will delete chapters 6-30, 25 chapters and their text files\") and wait for the user's explicit confirmation; then on the next turn call the tool with confirm=true.\n")
	sb.WriteString("4. Never widen the delete range \"for cleanliness\" or \"to make regeneration easier\". Prefer doing less to doing too much.\n")
	sb.WriteString("5. When user intent is ambiguous, ask a clarifying question instead of guessing into a write operation.\n\n")

	sb.WriteString("## Tool-selection guidance\n")
	sb.WriteString("- Tweak chapter content -> revise_chapter(num, feedback)\n")
	sb.WriteString("- Edit a pending chapter's outline -> edit_chapter_outline(num, title, outline)\n")
	sb.WriteString("- Give feedback on the overall outline -> revise_outline(feedback) (only touches unconfirmed chapters)\n")
	sb.WriteString("- Generate the next chapter's prose -> generate_chapter\n")
	sb.WriteString("- Confirmed chapters exist and the user wants to append more -> do NOT use generate_outline (it will be rejected); tell the user to use \"Generate Continuation Outline\" on the Outline page\n\n")

	sb.WriteString("## Important rules\n")
	sb.WriteString("- Async tools (generate_outline, generate_chapter, etc.) return \"task started\" immediately; results are pushed to the UI via logs. You MUST call the tool first and tell the user it has started only after receiving the tool result. Never output \"please wait\", \"hold on\", \"generating now\", or similar text without actually calling a tool — if you cannot fulfil the request, just explain why.\n")
	sb.WriteString("- When the user submits a story-config update (e.g. \"please update the following story config\"), use update_project_config.\n")
	sb.WriteString("- When the user submits a writing-style or synopsis update (e.g. \"please update writing style:\" or \"please update synopsis:\"), use update_project_config to save the corresponding field.\n")
	sb.WriteString("- When the user asks you to create/edit characters, worldview, etc., use the corresponding tool directly.\n")
	sb.WriteString("- When the user asks for outline/chapter generation, use the corresponding tool. If async, tell the user to wait.\n")
	sb.WriteString("- Before generating the outline, remind the user to check the Config page (story type, writing style, synopsis, characters, worldview) and confirm everything looks right.\n")
	sb.WriteString("- Before kicking off actual writing (confirming the outline), remind the user once more to confirm all settings, including character details and worldview entries.\n")
	sb.WriteString("- Before a write operation, prefer reading first (read_outline, read_chapter, etc.) to confirm the target exists and is in the expected state.\n")
	sb.WriteString("- After any operation, briefly report the result and suggest 1-2 next actions (e.g. check character settings, generate the outline, confirm the chapter) to help the user advance the project.\n")

	return sb.String()
}

func buildToolDescriptions(tools []Tool) string {
	var sb strings.Builder
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n  参数: %s\n", t.Name, t.Description, t.Parameters))
	}
	return sb.String()
}

func parseToolCall(content string) *ToolCall {
	content = strings.TrimSpace(content)

	idx := strings.Index(content, "<tool_call>")
	if idx == -1 {
		if tc := parseToolCallFunctionName(content); tc != nil {
			return tc
		}
		return parseToolCallJSON(content)
	}

	endIdx := strings.Index(content[idx:], "</tool_call>")
	if endIdx == -1 {
		// 标签未闭合，fallback 到全局搜索
		if tc := parseToolCallFunctionName(content); tc != nil {
			return tc
		}
		return parseToolCallJSON(content)
	}

	inner := strings.TrimSpace(content[idx+len("<tool_call>") : idx+endIdx])

	// 优先尝试直接 JSON 解析
	if tc := parseToolCallFromJSON(inner); tc != nil {
		return tc
	}

	// 尝试 XML 格式解析（<name>...</name> + <arguments>...</arguments>）
	if tc := parseToolCallFromXML(inner); tc != nil {
		return tc
	}

	// 标签内解析失败，fallback：在 </tool_call> 之后继续搜索 JSON
	remaining := content[idx+endIdx+len("</tool_call>"):]
	if tc := parseToolCallJSON(remaining); tc != nil {
		return tc
	}

	// 最终 fallback：在全部内容中搜索 JSON 工具调用
	if tc := parseToolCallJSON(content); tc != nil {
		return tc
	}

	if tc := parseToolCallFunctionName(content); tc != nil {
		return tc
	}

	return nil
}

func parseToolCallFunctionName(content string) *ToolCall {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "function.") {
			continue
		}
		rest := strings.TrimPrefix(line, "function.")
		parenIdx := strings.Index(rest, "(")
		if parenIdx == -1 {
			continue
		}
		name := rest[:parenIdx]
		if name == "" {
			continue
		}
		argsStr := strings.TrimSpace(rest[parenIdx+1:])
		argsStr = strings.TrimSuffix(argsStr, ")")
		argsStr = strings.TrimSpace(argsStr)
		if argsStr == "" {
			argsStr = "{}"
		}
		var args json.RawMessage
		if json.Unmarshal([]byte(argsStr), &args) != nil {
			args = json.RawMessage("{}")
		}
		return &ToolCall{Name: name, Arguments: args}
	}
	return nil
}

func parseToolCallFromXML(inner string) *ToolCall {
	// Parse XML format: <name>tool_name</name><arguments>{json}</arguments>
	nameStart := strings.Index(inner, "<name>")
	nameEnd := strings.Index(inner, "</name>")
	if nameStart == -1 || nameEnd == -1 || nameEnd <= nameStart {
		return nil
	}
	name := strings.TrimSpace(inner[nameStart+len("<name>") : nameEnd])
	if name == "" {
		return nil
	}

	args := json.RawMessage("{}")
	argsStart := strings.Index(inner, "<arguments>")
	argsEnd := strings.Index(inner, "</arguments>")
	if argsStart != -1 && argsEnd != -1 && argsEnd > argsStart {
		argsStr := strings.TrimSpace(inner[argsStart+len("<arguments>") : argsEnd])
		if argsStr != "" {
			var parsed json.RawMessage
			if json.Unmarshal([]byte(argsStr), &parsed) == nil {
				args = parsed
			}
		}
	}

	return &ToolCall{Name: name, Arguments: args}
}

func parseToolCallJSON(content string) *ToolCall {
	// Try all JSON objects in the content, not just the first one
	remaining := content
	for {
		start := strings.Index(remaining, "{")
		if start == -1 {
			return nil
		}
		remaining = remaining[start:]

		jsonStr := extractJSON(remaining)
		if jsonStr == "" {
			return nil
		}

		tc := parseToolCallFromJSON(jsonStr)
		if tc != nil {
			return tc
		}

		// Move past this JSON object to try the next one
		remaining = remaining[len(jsonStr):]
	}
}

func parseToolCallFromJSON(jsonStr string) *ToolCall {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil
	}

	nameRaw, ok := raw["name"]
	if !ok {
		nameRaw, ok = raw["tool"]
	}
	if !ok {
		return nil
	}

	var name string
	if err := json.Unmarshal(nameRaw, &name); err != nil {
		return nil
	}

	args, _ := json.Marshal(raw["arguments"])
	if args == nil {
		args = json.RawMessage("{}")
	}

	return &ToolCall{Name: name, Arguments: args}
}

func extractJSON(content string) string {
	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}

	return ""
}

func executeTool(call *ToolCall, tools []Tool, ctx *AgentContext) string {
	for _, t := range tools {
		if t.Name == call.Name {
			result, err := t.Execute(call.Arguments, ctx)
			if err != nil {
				return fmt.Sprintf("工具执行错误: %v", err)
			}
			return result
		}
	}
	return fmt.Sprintf("未知工具: %s", call.Name)
}

// requireConfirm 检查危险操作的 confirm 参数。
// 未确认时返回非空提示（作为工具结果反馈给 AI，要求其先征得用户同意）。
func requireConfirm(args json.RawMessage, action string) string {
	var params struct {
		Confirm bool `json:"confirm"`
	}
	json.Unmarshal(args, &params)
	if params.Confirm {
		return ""
	}
	return fmt.Sprintf("⚠️ 操作未执行：「%s」是不可逆的危险操作。请先向用户复述影响范围并获得明确同意，确认后携带 confirm=true 重新调用。如果用户的本意是修改内容而非删除，请改用对应的修订工具。", action)
}

func getBuiltinTools() []Tool {
	return []Tool{
		{
			Name:        "read_characters",
			Description: "获取角色列表，可按名称过滤",
			Parameters:  `{"filter": "可选，按名称过滤"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Filter string `json:"filter"`
				}
				json.Unmarshal(args, &params)

				if ctx.Settings == nil {
					return "暂无角色数据", nil
				}

				var result strings.Builder
				for _, c := range ctx.Settings.Characters {
					if params.Filter != "" && !strings.Contains(c.Name, params.Filter) {
						continue
					}
					result.WriteString(fmt.Sprintf("【%s】(ID:%s)\n", c.Name, c.ID))
					if c.Age != "" {
						result.WriteString(fmt.Sprintf("  年龄: %s\n", c.Age))
					}
					if c.Personality != "" {
						result.WriteString(fmt.Sprintf("  性格: %s\n", c.Personality))
					}
					if c.Background != "" {
						result.WriteString(fmt.Sprintf("  背景: %s\n", c.Background))
					}
					result.WriteString("\n")
				}

				if result.Len() == 0 {
					return "没有找到匹配的角色", nil
				}
				return result.String(), nil
			},
		},
		{
			Name:        "read_character",
			Description: "获取单个角色详情，通过ID或名称",
			Parameters:  `{"id": "角色ID或名称"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID string `json:"id"`
				}
				json.Unmarshal(args, &params)

				if ctx.Settings == nil {
					return "暂无角色数据", nil
				}

				for _, c := range ctx.Settings.Characters {
					if c.ID == params.ID || c.Name == params.ID {
						data, _ := json.MarshalIndent(c, "", "  ")
						return string(data), nil
					}
				}
				return fmt.Sprintf("未找到角色: %s", params.ID), nil
			},
		},
		{
			Name:        "read_worldview",
			Description: "获取世界观条目列表，可按分类过滤",
			Parameters:  `{"category": "可选分类: geography/faction/rule/history/other"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Category string `json:"category"`
				}
				json.Unmarshal(args, &params)

				if ctx.Settings == nil || len(ctx.Settings.Worldview) == 0 {
					return "暂无世界观数据", nil
				}

				var result strings.Builder
				for _, w := range ctx.Settings.Worldview {
					if params.Category != "" && w.Category != params.Category {
						continue
					}
					result.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n\n", w.Name, w.Category, w.Description))
				}

				if result.Len() == 0 {
					return "没有找到匹配的世界观条目", nil
				}
				return result.String(), nil
			},
		},
		{
			Name:        "read_organizations",
			Description: "获取组织列表",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if ctx.Settings == nil || len(ctx.Settings.Organizations) == 0 {
					return "暂无组织数据", nil
				}

				var result strings.Builder
				for _, o := range ctx.Settings.Organizations {
					result.WriteString(fmt.Sprintf("【%s】(ID:%s, 类型:%s)\n  %s\n", o.Name, o.ID, o.Type, o.Description))
					if len(o.Members) > 0 {
						result.WriteString(fmt.Sprintf("  成员IDs: %s\n", strings.Join(o.Members, ", ")))
					}
					result.WriteString("\n")
				}
				return result.String(), nil
			},
		},
		{
			Name:        "read_chapter",
			Description: "获取指定章节内容",
			Parameters:  `{"num": 1}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Num int `json:"num"`
				}
				json.Unmarshal(args, &params)

				for _, ch := range ctx.State.Chapters {
					if ch.Num == params.Num {
						var result strings.Builder
						result.WriteString(fmt.Sprintf("第%d章《%s》[%s]\n\n", ch.Num, ch.Title, ch.Status))
						if ch.Outline != "" {
							result.WriteString(fmt.Sprintf("大纲: %s\n\n", ch.Outline))
						}
						if ch.Summary != "" {
							result.WriteString(fmt.Sprintf("摘要: %s\n\n", ch.Summary))
						}
						if ch.Content != "" {
							result.WriteString(ch.Content)
						} else {
							result.WriteString("(尚未生成内容)")
						}
						return result.String(), nil
					}
				}
				return fmt.Sprintf("未找到第%d章", params.Num), nil
			},
		},
		{
			Name:        "read_outline",
			Description: "获取完整大纲",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if len(ctx.State.Chapters) == 0 {
					return "暂无大纲", nil
				}

				var result strings.Builder
				result.WriteString(fmt.Sprintf("《%s》\n\n", ctx.State.Title))
				for _, ch := range ctx.State.Chapters {
					status := ""
					switch ch.Status {
					case StatusAccepted:
						status = "✅"
					case StatusReview:
						status = "👀"
					case StatusWriting:
						status = "⏳"
					}
					result.WriteString(fmt.Sprintf("第%d章 %s《%s》: %s\n", ch.Num, status, ch.Title, ch.Outline))
				}
				return result.String(), nil
			},
		},
		{
			Name:        "read_foreshadows",
			Description: "获取伏笔列表",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if len(ctx.State.Foreshadows) == 0 {
					return "暂无伏笔", nil
				}

				var result strings.Builder
				for _, fs := range ctx.State.Foreshadows {
					result.WriteString(fmt.Sprintf("#%d [%s] %s\n", fs.ID, foreshadowStatusLabel(fs.Status), fs.Name))
					result.WriteString(fmt.Sprintf("  描述: %s\n", fs.Description))
					result.WriteString(fmt.Sprintf("  埋设: 第%d章", fs.PlantChapter))
					if fs.TargetChapter > 0 {
						result.WriteString(fmt.Sprintf(" → 预计回收: 第%d章", fs.TargetChapter))
					}
					result.WriteString("\n")
					if len(fs.Events) > 0 {
						result.WriteString("  进展:\n")
						for _, ev := range fs.Events {
							result.WriteString(fmt.Sprintf("    - 第%d章: %s\n", ev.Chapter, ev.Note))
						}
					}
					if fs.Resolution != "" {
						result.WriteString(fmt.Sprintf("  回收方式: %s\n", fs.Resolution))
					}
					result.WriteString("\n")
				}
				return result.String(), nil
			},
		},
		{
			Name:        "search_project",
			Description: "全文搜索项目数据（角色名、世界观、大纲等）",
			Parameters:  `{"query": "搜索关键词"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Query string `json:"query"`
				}
				json.Unmarshal(args, &params)

				if params.Query == "" {
					return "请提供搜索关键词", nil
				}

				var results []string
				q := strings.ToLower(params.Query)

				if ctx.Settings != nil {
					for _, c := range ctx.Settings.Characters {
						if strings.Contains(strings.ToLower(c.Name), q) || strings.Contains(strings.ToLower(c.Background), q) {
							results = append(results, fmt.Sprintf("[角色] %s: %s", c.Name, truncate(c.Background, 100)))
						}
					}
					for _, w := range ctx.Settings.Worldview {
						if strings.Contains(strings.ToLower(w.Name), q) || strings.Contains(strings.ToLower(w.Description), q) {
							results = append(results, fmt.Sprintf("[世界观] %s: %s", w.Name, truncate(w.Description, 100)))
						}
					}
				}

				for _, ch := range ctx.State.Chapters {
					if strings.Contains(strings.ToLower(ch.Title), q) || strings.Contains(strings.ToLower(ch.Outline), q) {
						results = append(results, fmt.Sprintf("[章节] 第%d章《%s》: %s", ch.Num, ch.Title, truncate(ch.Outline, 100)))
					}
				}

				if len(results) == 0 {
					return "未找到相关内容", nil
				}
				return strings.Join(results, "\n"), nil
			},
		},
		{
			Name:        "create_character",
			Description: "创建新角色",
			Parameters:  `{"name": "角色名", "age": "", "appearance": "", "personality": "", "background": "", "motivation": "", "abilities": "", "notes": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var c Character
				if err := json.Unmarshal(args, &c); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if c.Name == "" {
					return "", fmt.Errorf("角色名不能为空")
				}

				c.ID = ctx.Settings.nextCharacterID()
				ctx.Settings.Characters = append(ctx.Settings.Characters, c)

				if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				if ctx.Logger != nil {
					ctx.Logger.SettingsUpdated()
				}

				return fmt.Sprintf("角色「%s」创建成功 (ID: %s)", c.Name, c.ID), nil
			},
		},
		{
			Name:        "update_character",
			Description: "更新角色信息",
			Parameters:  `{"id": "角色ID", "name": "", "age": "", "personality": "", "background": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Age         string `json:"age"`
					Appearance  string `json:"appearance"`
					Personality string `json:"personality"`
					Background  string `json:"background"`
					Motivation  string `json:"motivation"`
					Abilities   string `json:"abilities"`
					Notes       string `json:"notes"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}

				for i, c := range ctx.Settings.Characters {
					if c.ID == params.ID || c.Name == params.ID {
						if params.Name != "" {
							ctx.Settings.Characters[i].Name = params.Name
						}
						if params.Age != "" {
							ctx.Settings.Characters[i].Age = params.Age
						}
						if params.Appearance != "" {
							ctx.Settings.Characters[i].Appearance = params.Appearance
						}
						if params.Personality != "" {
							ctx.Settings.Characters[i].Personality = params.Personality
						}
						if params.Background != "" {
							ctx.Settings.Characters[i].Background = params.Background
						}
						if params.Motivation != "" {
							ctx.Settings.Characters[i].Motivation = params.Motivation
						}
						if params.Abilities != "" {
							ctx.Settings.Characters[i].Abilities = params.Abilities
						}
						if params.Notes != "" {
							ctx.Settings.Characters[i].Notes = params.Notes
						}

						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}

						return fmt.Sprintf("角色「%s」已更新", ctx.Settings.Characters[i].Name), nil
					}
				}
				return fmt.Sprintf("未找到角色: %s", params.ID), nil
			},
		},
		{
			Name:        "delete_character",
			Description: "删除角色",
			Parameters:  `{"id": "角色ID"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID string `json:"id"`
				}
				json.Unmarshal(args, &params)

				for i, c := range ctx.Settings.Characters {
					if c.ID == params.ID || c.Name == params.ID {
						ctx.Settings.Characters = append(ctx.Settings.Characters[:i], ctx.Settings.Characters[i+1:]...)
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return fmt.Sprintf("角色「%s」已删除", c.Name), nil
					}
				}
				return fmt.Sprintf("未找到角色: %s", params.ID), nil
			},
		},
		{
			Name:        "create_worldview",
			Description: "创建世界观条目",
			Parameters:  `{"name": "名称", "category": "分类", "description": "描述", "tags": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var w WorldviewEntry
				if err := json.Unmarshal(args, &w); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if w.Name == "" || w.Description == "" {
					return "", fmt.Errorf("名称和描述不能为空")
				}

				w.ID = ctx.Settings.nextWorldviewID()
				ctx.Settings.Worldview = append(ctx.Settings.Worldview, w)

				if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				if ctx.Logger != nil {
					ctx.Logger.SettingsUpdated()
				}

				return fmt.Sprintf("世界观条目「%s」创建成功 (ID: %s)", w.Name, w.ID), nil
			},
		},
		{
			Name:        "update_worldview",
			Description: "更新世界观条目",
			Parameters:  `{"id": "条目ID", "name": "", "category": "", "description": "", "tags": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Category    string `json:"category"`
					Description string `json:"description"`
					Tags        string `json:"tags"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}

				for i, w := range ctx.Settings.Worldview {
					if w.ID == params.ID || w.Name == params.ID {
						if params.Name != "" {
							ctx.Settings.Worldview[i].Name = params.Name
						}
						if params.Category != "" {
							ctx.Settings.Worldview[i].Category = params.Category
						}
						if params.Description != "" {
							ctx.Settings.Worldview[i].Description = params.Description
						}
						if params.Tags != "" {
							ctx.Settings.Worldview[i].Tags = params.Tags
						}

						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}

						return fmt.Sprintf("世界观条目「%s」已更新", ctx.Settings.Worldview[i].Name), nil
					}
				}
				return fmt.Sprintf("未找到世界观条目: %s", params.ID), nil
			},
		},
		{
			Name:        "delete_worldview",
			Description: "删除世界观条目",
			Parameters:  `{"id": "条目ID"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID string `json:"id"`
				}
				json.Unmarshal(args, &params)

				for i, w := range ctx.Settings.Worldview {
					if w.ID == params.ID || w.Name == params.ID {
						ctx.Settings.Worldview = append(ctx.Settings.Worldview[:i], ctx.Settings.Worldview[i+1:]...)
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return fmt.Sprintf("世界观条目「%s」已删除", w.Name), nil
					}
				}
				return fmt.Sprintf("未找到世界观条目: %s", params.ID), nil
			},
		},
		{
			Name:        "read_project_config",
			Description: "读取当前故事配置",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				snapshot := ctx.State.StoryConfigSnapshot
				if snapshot == nil {
					snapshot = &ctx.Config.Story
				}
				data, _ := json.MarshalIndent(snapshot, "", "  ")
				return string(data), nil
			},
		},
		{
			Name:        "update_project_config",
			Description: "更新故事配置。如果存在已确认章节，会自动触发设定协调。",
			Parameters:  `{"type": "故事类型", "title": "标题", "chapter_count": 30, "target_words_per_chapter": 2500, "writing_style": "写作风格", "story_synopsis": "故事梗概"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Type                  string `json:"type"`
					Title                 string `json:"title"`
					ChapterCount          int    `json:"chapter_count"`
					TargetWordsPerChapter int    `json:"target_words_per_chapter"`
					WritingStyle          string `json:"writing_style"`
					StorySynopsis         string `json:"story_synopsis"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}

				if params.Type != "" {
					ctx.Config.Story.Type = params.Type
				}
				if params.Title != "" {
					ctx.Config.Story.Title = params.Title
				}
				if params.ChapterCount > 0 {
					ctx.Config.Story.ChapterCount = params.ChapterCount
				}
				if params.TargetWordsPerChapter > 0 {
					ctx.Config.Story.TargetWordsPerChapter = params.TargetWordsPerChapter
				}
				if params.WritingStyle != "" {
					ctx.Config.Story.WritingStyle = params.WritingStyle
				}
				if params.StorySynopsis != "" {
					ctx.Config.Story.StorySynopsis = params.StorySynopsis
				}

				if err := saveConfig(ctx.CfgPath, ctx.Config); err != nil {
					return "", fmt.Errorf("保存配置失败: %w", err)
				}

				hasAccepted := false
				for _, ch := range ctx.State.Chapters {
					if ch.Status == StatusAccepted {
						hasAccepted = true
						break
					}
				}

				if hasAccepted && ctx.StartAsync != nil {
					newSettings := ctx.Config.Story
					ctx.StartAsync("settings_reconciliation", func(goCtx context.Context) {
						err := ReconcileSettingsAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, newSettings, ctx.ProgressPath, ctx.CfgPath, ctx.Logger)
						if err != nil {
							ctx.Logger.Error(fmt.Sprintf("设定协调失败: %v", err))
							return
						}
						ctx.Logger.Success("设定协调完成！")
					})
					return "故事配置已保存，正在自动协调已有内容...", nil
				}

				if ctx.Logger != nil {
					ctx.Logger.SettingsUpdated()
				}
				return "故事配置已保存", nil
			},
		},
		{
			Name:        "generate_outline",
			Description: "生成小说大纲（异步）。注意：存在已确认章节时不可用（会覆盖已完成内容），追加章节请引导用户使用大纲页的「生成后续大纲」。",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if ctx.StartAsync == nil {
					return "", fmt.Errorf("异步任务系统未初始化")
				}
				for _, ch := range ctx.State.Chapters {
					if ch.Status == StatusAccepted {
						return "", fmt.Errorf("存在已确认章节，无法整体重新生成大纲（会覆盖已完成内容）。请引导用户使用「生成后续大纲」追加章节")
					}
					if ch.Status == StatusWriting || ch.Status == StatusReview {
						return "", fmt.Errorf("有正在写作/审核中的章节，请先处理后再重新生成大纲")
					}
				}
				ctx.StartAsync("outline_generation", func(goCtx context.Context) {
					err := GenerateOutlineAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.ProgressPath, ctx.Logger)
					if err != nil {
						ctx.Logger.Error(fmt.Sprintf("大纲生成失败: %v", err))
						return
					}
					ctx.Logger.Success("大纲生成完成！")
				})
				return "大纲生成任务已启动，请等待完成。", nil
			},
		},
		{
			Name:        "confirm_outline",
			Description: "确认大纲，进入写作阶段",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if ctx.State.Phase != "outline" {
					return "", fmt.Errorf("当前不在大纲阶段")
				}
				if len(ctx.State.Chapters) == 0 {
					return "", fmt.Errorf("大纲为空，请先生成大纲")
				}
				if err := ConfirmOutlineAction(ctx.State, ctx.ProgressPath); err != nil {
					return "", fmt.Errorf("确认大纲失败: %w", err)
				}
				ctx.Logger.Success("大纲已确认，进入写作阶段。")
				return "大纲已确认，现在进入写作阶段。", nil
			},
		},
		{
			Name:        "revise_outline",
			Description: "根据反馈修订大纲（异步）",
			Parameters:  `{"feedback": "修改意见"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Feedback string `json:"feedback"`
				}
				if err := json.Unmarshal(args, &params); err != nil || params.Feedback == "" {
					return "", fmt.Errorf("缺少 feedback 参数")
				}
				if ctx.StartAsync == nil {
					return "", fmt.Errorf("异步任务系统未初始化")
				}
				feedback := params.Feedback
				ctx.StartAsync("outline_revision", func(goCtx context.Context) {
					err := ReviseOutlineAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.ProgressPath, feedback, ctx.Logger)
					if err != nil {
						ctx.Logger.Error(fmt.Sprintf("大纲修订失败: %v", err))
						return
					}
					ctx.Logger.Success("大纲已修订。")
				})
				return "大纲修订任务已启动，请等待完成。", nil
			},
		},
		{
			Name:        "delete_outline",
			Description: "【危险·不可逆】删除整个大纲及全部章节数据。仅当用户明确要求删除大纲时使用，且必须先向用户确认。严禁用于实现「修改大纲」的需求——修改请用 revise_outline 或 edit_chapter_outline。",
			Parameters:  `{"confirm": true}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if msg := requireConfirm(args, fmt.Sprintf("删除整个大纲（共 %d 章）", len(ctx.State.Chapters))); msg != "" {
					return msg, nil
				}
				for _, ch := range ctx.State.Chapters {
					if ch.Status == StatusWriting || ch.Status == StatusReview {
						return "", fmt.Errorf("有正在写作/审核中的章节，请先处理后再删除大纲")
					}
				}
				ctx.State.Title = ""
				ctx.State.CorePrompt = ""
				ctx.State.StorySynopsis = ""
				ctx.State.Chapters = nil
				ctx.State.StoryConfigSnapshot = nil
				ctx.State.CurrentChapterIndex = 0
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存进度失败: %w", err)
				}
				ctx.Logger.Success("大纲已删除。")
				return "大纲已删除。", nil
			},
		},
		{
			Name:        "edit_chapter_outline",
			Description: "编辑指定章节的标题和大纲（仅 pending 状态可编辑）",
			Parameters:  `{"num": 1, "title": "新标题", "outline": "新大纲"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Num     int    `json:"num"`
					Title   string `json:"title"`
					Outline string `json:"outline"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if err := EditChapterOutline(ctx.State, params.Num, params.Title, params.Outline); err != nil {
					return "", err
				}
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存进度失败: %w", err)
				}
				ctx.Logger.Success(fmt.Sprintf("第 %d 章大纲已更新。", params.Num))
				return fmt.Sprintf("第 %d 章大纲已更新。", params.Num), nil
			},
		},
		{
			Name:        "generate_chapter",
			Description: "生成当前章节内容（异步）",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if ctx.State.Phase != "writing" {
					return "", fmt.Errorf("当前不在写作阶段")
				}
				if ctx.StartAsync == nil {
					return "", fmt.Errorf("异步任务系统未初始化")
				}
				chIdx := ctx.State.CurrentChapterIndex
				chTitle := ""
				if chIdx < len(ctx.State.Chapters) {
					chTitle = ctx.State.Chapters[chIdx].Title
				}
				ctx.StartAsync("chapter_generation", func(goCtx context.Context) {
					err := GenerateChapterAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.ProgressPath, ctx.Settings, ctx.Logger)
					if err != nil {
						ctx.Logger.Error(fmt.Sprintf("章节创作失败: %v", err))
						return
					}
					ctx.Logger.Success(fmt.Sprintf("第 %d 章《%s》创作完成！", chIdx+1, chTitle))
				})
				return fmt.Sprintf("第 %d 章生成任务已启动，请等待完成。", chIdx+1), nil
			},
		},
		{
			Name:        "confirm_chapter",
			Description: "确认当前章节",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if ctx.State.Phase != "writing" {
					return "", fmt.Errorf("当前不在写作阶段")
				}
				if err := ConfirmChapterAction(ctx.State, ctx.ProgressPath); err != nil {
					return "", err
				}
				ch := ctx.State.Chapters[ctx.State.CurrentChapterIndex-1]
				ctx.Logger.Success(fmt.Sprintf("第 %d 章已确认。", ch.Num))
				return fmt.Sprintf("第 %d 章《%s》已确认。", ch.Num, ch.Title), nil
			},
		},
		{
			Name:        "revise_chapter",
			Description: "根据反馈修订章节正文（异步）。通过 num 指定要修订的章节号（可以是任意已有内容的章节，包括已确认章节）；省略 num 则修订当前写作中的章节。这是修改章节内容的唯一正确方式：只改动目标章节本身，不影响其他章节和大纲。",
			Parameters:  `{"num": 6, "feedback": "具体修改意见"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Num      int    `json:"num"`
					Feedback string `json:"feedback"`
				}
				if err := json.Unmarshal(args, &params); err != nil || strings.TrimSpace(params.Feedback) == "" {
					return "", fmt.Errorf("缺少 feedback 参数")
				}
				if ctx.StartAsync == nil {
					return "", fmt.Errorf("异步任务系统未初始化")
				}
				feedback := params.Feedback
				num := params.Num

				// 未指定章节号 → 修订当前章节（写作流程内）
				if num <= 0 {
					if ctx.State.Phase != "writing" || ctx.State.CurrentChapterIndex >= len(ctx.State.Chapters) {
						return "", fmt.Errorf("未指定章节号且当前没有写作中的章节，请通过 num 参数指定要修订的章节")
					}
					num = ctx.State.Chapters[ctx.State.CurrentChapterIndex].Num
				}

				// 校验目标章节
				var target *ChapterState
				for i := range ctx.State.Chapters {
					if ctx.State.Chapters[i].Num == num {
						target = &ctx.State.Chapters[i]
						break
					}
				}
				if target == nil {
					return "", fmt.Errorf("第 %d 章不存在", num)
				}
				if target.Content == "" {
					return "", fmt.Errorf("第 %d 章尚未生成内容，无法修订", num)
				}

				// 当前审核中的章节走完整修订流程（含后续大纲联动），
				// 其他章节走定向最小化修订（零副作用）。
				isCurrent := ctx.State.Phase == "writing" &&
					ctx.State.CurrentChapterIndex < len(ctx.State.Chapters) &&
					ctx.State.Chapters[ctx.State.CurrentChapterIndex].Num == num &&
					(target.Status == StatusReview || target.Status == StatusWriting)

				chNum := num
				ctx.StartAsync("chapter_revision", func(goCtx context.Context) {
					var err error
					if isCurrent {
						err = ReviseChapterAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.ProgressPath, feedback, ctx.Settings, ctx.Logger)
					} else {
						err = ReviseSpecificChapterAction(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.ProgressPath, chNum, feedback, ctx.Settings, ctx.Logger)
					}
					if err != nil {
						ctx.Logger.Error(fmt.Sprintf("章节修订失败: %v", err))
						return
					}
				})
				return fmt.Sprintf("第 %d 章修订任务已启动（仅修改该章，不影响其他章节），请等待完成。", num), nil
			},
		},
		{
			Name:        "delete_chapter",
			Description: "【危险·不可逆】删除最后一个章节。仅当用户明确要求删除时使用，且必须先向用户确认。",
			Parameters:  `{"confirm": true}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if msg := requireConfirm(args, "删除最后一个章节"); msg != "" {
					return msg, nil
				}
				if len(ctx.State.Chapters) == 0 {
					return "", fmt.Errorf("没有可删除的章节")
				}
				lastIdx := len(ctx.State.Chapters) - 1
				ch := ctx.State.Chapters[lastIdx]
				if ch.Status == StatusWriting {
					return "", fmt.Errorf("正在写作中的章节无法删除")
				}
				deleteFile(ChapterMarkdownPath(ctx.ProjectDir, ch.Num))
				ctx.State.Chapters = ctx.State.Chapters[:lastIdx]
				if ctx.State.CurrentChapterIndex > len(ctx.State.Chapters) {
					ctx.State.CurrentChapterIndex = len(ctx.State.Chapters)
				}
				if len(ctx.State.Chapters) == 0 {
					ctx.State.Phase = "outline"
					ctx.State.CurrentChapterIndex = 0
					ctx.State.StoryConfigSnapshot = nil
				}
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存进度失败: %w", err)
				}
				ctx.Logger.Success(fmt.Sprintf("已删除第 %d 章。", ch.Num))
				return fmt.Sprintf("已删除第 %d 章。", ch.Num), nil
			},
		},
		{
			Name:        "delete_chapters_from",
			Description: "【危险·不可逆】从指定章节删除到末尾，将永久删除范围内所有章节的大纲和正文。仅当用户明确要求批量删除时使用，且必须先向用户复述删除范围并获得确认。严禁用于实现「修改某章」的需求——修改请用 revise_chapter。",
			Parameters:  `{"num": 6, "confirm": true}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					Num     int  `json:"num"`
					Confirm bool `json:"confirm"`
				}
				json.Unmarshal(args, &params)

				if !params.Confirm {
					affected := 0
					for _, ch := range ctx.State.Chapters {
						if ch.Num >= params.Num {
							affected++
						}
					}
					return fmt.Sprintf("⚠️ 操作未执行：这将永久删除第 %d 章到末尾共 %d 章的全部内容。请先向用户复述此影响范围并获得明确同意，确认后携带 confirm=true 重新调用。如果用户的本意是修改章节内容，请改用 revise_chapter。", params.Num, affected), nil
				}

				startIdx := -1
				for i, ch := range ctx.State.Chapters {
					if ch.Num == params.Num {
						startIdx = i
						break
					}
				}
				if startIdx == -1 {
					return "", fmt.Errorf("章节 %d 不存在", params.Num)
				}
				for i := startIdx; i < len(ctx.State.Chapters); i++ {
					if ctx.State.Chapters[i].Status == StatusWriting {
						return "", fmt.Errorf("删除范围内有正在写作中的章节，无法删除")
					}
				}
				deletedCount := len(ctx.State.Chapters) - startIdx
				for i := startIdx; i < len(ctx.State.Chapters); i++ {
					deleteFile(ChapterMarkdownPath(ctx.ProjectDir, ctx.State.Chapters[i].Num))
				}
				ctx.State.Chapters = ctx.State.Chapters[:startIdx]
				if ctx.State.CurrentChapterIndex > len(ctx.State.Chapters) {
					ctx.State.CurrentChapterIndex = len(ctx.State.Chapters)
				}
				if len(ctx.State.Chapters) == 0 {
					ctx.State.Phase = "outline"
					ctx.State.CurrentChapterIndex = 0
					ctx.State.StoryConfigSnapshot = nil
				}
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存进度失败: %w", err)
				}
				ctx.Logger.Success(fmt.Sprintf("已从第 %d 章删除到末尾，共删除 %d 章。", params.Num, deletedCount))
				return fmt.Sprintf("已从第 %d 章删除到末尾，共删除 %d 章。", params.Num, deletedCount), nil
			},
		},
		{
			Name:        "create_organization",
			Description: "创建组织",
			Parameters:  `{"name": "组织名", "type": "类型", "description": "描述", "members": ["成员ID"]}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var o Organization
				if err := json.Unmarshal(args, &o); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if o.Name == "" {
					return "", fmt.Errorf("组织名不能为空")
				}
				o.ID = ctx.Settings.nextOrganizationID()
				ctx.Settings.Organizations = append(ctx.Settings.Organizations, o)
				if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				if ctx.Logger != nil {
					ctx.Logger.SettingsUpdated()
				}
				return fmt.Sprintf("组织「%s」创建成功 (ID: %s)", o.Name, o.ID), nil
			},
		},
		{
			Name:        "update_organization",
			Description: "更新组织信息",
			Parameters:  `{"id": "组织ID", "name": "", "type": "", "description": "", "members": []}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID          string   `json:"id"`
					Name        string   `json:"name"`
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Members     []string `json:"members"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				for i, o := range ctx.Settings.Organizations {
					if o.ID == params.ID || o.Name == params.ID {
						if params.Name != "" {
							ctx.Settings.Organizations[i].Name = params.Name
						}
						if params.Type != "" {
							ctx.Settings.Organizations[i].Type = params.Type
						}
						if params.Description != "" {
							ctx.Settings.Organizations[i].Description = params.Description
						}
						if params.Members != nil {
							ctx.Settings.Organizations[i].Members = params.Members
						}
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return fmt.Sprintf("组织「%s」已更新", ctx.Settings.Organizations[i].Name), nil
					}
				}
				return fmt.Sprintf("未找到组织: %s", params.ID), nil
			},
		},
		{
			Name:        "delete_organization",
			Description: "删除组织",
			Parameters:  `{"id": "组织ID"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID string `json:"id"`
				}
				json.Unmarshal(args, &params)
				for i, o := range ctx.Settings.Organizations {
					if o.ID == params.ID || o.Name == params.ID {
						ctx.Settings.Organizations = append(ctx.Settings.Organizations[:i], ctx.Settings.Organizations[i+1:]...)
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return fmt.Sprintf("组织「%s」已删除", o.Name), nil
					}
				}
				return fmt.Sprintf("未找到组织: %s", params.ID), nil
			},
		},
		{
			Name:        "create_relation",
			Description: "创建关系",
			Parameters:  `{"source_id": "源ID", "source_type": "源类型", "target_id": "目标ID", "target_type": "目标类型", "label": "关系标签"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var rel Relation
				if err := json.Unmarshal(args, &rel); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if rel.SourceID == "" || rel.TargetID == "" {
					return "", fmt.Errorf("源和目标不能为空")
				}
				rel.ID = ctx.Settings.nextRelationID()
				ctx.Settings.Relations = append(ctx.Settings.Relations, rel)
				if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				if ctx.Logger != nil {
					ctx.Logger.SettingsUpdated()
				}
				return fmt.Sprintf("关系创建成功 (ID: %s)", rel.ID), nil
			},
		},
		{
			Name:        "update_relation",
			Description: "更新关系",
			Parameters:  `{"id": "关系ID", "source_id": "", "source_type": "", "target_id": "", "target_type": "", "label": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID         string `json:"id"`
					SourceID   string `json:"source_id"`
					SourceType string `json:"source_type"`
					TargetID   string `json:"target_id"`
					TargetType string `json:"target_type"`
					Label      string `json:"label"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				for i, rel := range ctx.Settings.Relations {
					if rel.ID == params.ID {
						if params.SourceID != "" {
							ctx.Settings.Relations[i].SourceID = params.SourceID
						}
						if params.SourceType != "" {
							ctx.Settings.Relations[i].SourceType = params.SourceType
						}
						if params.TargetID != "" {
							ctx.Settings.Relations[i].TargetID = params.TargetID
						}
						if params.TargetType != "" {
							ctx.Settings.Relations[i].TargetType = params.TargetType
						}
						if params.Label != "" {
							ctx.Settings.Relations[i].Label = params.Label
						}
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return fmt.Sprintf("关系已更新 (ID: %s)", ctx.Settings.Relations[i].ID), nil
					}
				}
				return fmt.Sprintf("未找到关系: %s", params.ID), nil
			},
		},
		{
			Name:        "delete_relation",
			Description: "删除关系",
			Parameters:  `{"id": "关系ID"}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID string `json:"id"`
				}
				json.Unmarshal(args, &params)
				for i, rel := range ctx.Settings.Relations {
					if rel.ID == params.ID {
						ctx.Settings.Relations = append(ctx.Settings.Relations[:i], ctx.Settings.Relations[i+1:]...)
						if err := SaveProjectSettings(ctx.SettingsPath, ctx.Settings); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						if ctx.Logger != nil {
							ctx.Logger.SettingsUpdated()
						}
						return "关系已删除", nil
					}
				}
				return fmt.Sprintf("未找到关系: %s", params.ID), nil
			},
		},
		{
			Name:        "suggest_foreshadows",
			Description: "AI 建议伏笔方案（异步）",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if len(ctx.State.Chapters) == 0 {
					return "", fmt.Errorf("请先生成大纲")
				}
				if ctx.StartAsync == nil {
					return "", fmt.Errorf("异步任务系统未初始化")
				}
				ctx.StartAsync("foreshadow_suggest", func(goCtx context.Context) {
					suggestions, err := SuggestForeshadows(goCtx, ctx.APICfg, ctx.Config, ctx.State, ctx.Logger)
					if err != nil {
						ctx.Logger.Error(fmt.Sprintf("伏笔建议生成失败: %v", err))
						return
					}
					ctx.Logger.Success(fmt.Sprintf("伏笔建议生成完成，共 %d 条", len(suggestions)))
					ctx.Logger.ForeshadowSuggestions(suggestions)
				})
				return "伏笔建议生成任务已启动，请等待完成。", nil
			},
		},
		{
			Name:        "create_foreshadow",
			Description: "创建伏笔",
			Parameters:  `{"name": "伏笔名", "description": "描述", "plant_chapter": 1, "target_chapter": 5}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var req struct {
					Name          string `json:"name"`
					Description   string `json:"description"`
					PlantChapter  int    `json:"plant_chapter"`
					TargetChapter int    `json:"target_chapter"`
				}
				if err := json.Unmarshal(args, &req); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				if req.Name == "" || req.Description == "" {
					return "", fmt.Errorf("名称和描述不能为空")
				}
				fs := Foreshadow{
					ID:            NextForeshadowID(ctx.State.Foreshadows),
					Name:          req.Name,
					Description:   req.Description,
					PlantChapter:  req.PlantChapter,
					TargetChapter: req.TargetChapter,
					Status:        ForeshadowPlanted,
					Events:        []ForeshadowEvent{},
				}
				ctx.State.Foreshadows = append(ctx.State.Foreshadows, fs)
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				_ = SaveForeshadowRoadmap(filepath.Dir(ctx.ProgressPath), ctx.State)
				return fmt.Sprintf("伏笔「%s」创建成功 (ID: %d)", fs.Name, fs.ID), nil
			},
		},
		{
			Name:        "update_foreshadow",
			Description: "更新伏笔",
			Parameters:  `{"id": 1, "name": "", "description": "", "plant_chapter": 0, "target_chapter": 0, "status": "", "resolution": ""}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var req struct {
					ID            int    `json:"id"`
					Name          string `json:"name"`
					Description   string `json:"description"`
					PlantChapter  int    `json:"plant_chapter"`
					TargetChapter int    `json:"target_chapter"`
					Status        string `json:"status"`
					Resolution    string `json:"resolution"`
				}
				if err := json.Unmarshal(args, &req); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				idx := -1
				for i, fs := range ctx.State.Foreshadows {
					if fs.ID == req.ID {
						idx = i
						break
					}
				}
				if idx == -1 {
					return "", fmt.Errorf("伏笔 %d 不存在", req.ID)
				}
				fs := &ctx.State.Foreshadows[idx]
				if req.Name != "" {
					fs.Name = req.Name
				}
				if req.Description != "" {
					fs.Description = req.Description
				}
				if req.PlantChapter > 0 {
					fs.PlantChapter = req.PlantChapter
				}
				if req.TargetChapter > 0 {
					fs.TargetChapter = req.TargetChapter
				}
				if req.Status != "" {
					fs.Status = ForeshadowStatus(req.Status)
				}
				if req.Resolution != "" {
					fs.Resolution = req.Resolution
				}
				if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
					return "", fmt.Errorf("保存失败: %w", err)
				}
				_ = SaveForeshadowRoadmap(filepath.Dir(ctx.ProgressPath), ctx.State)
				return fmt.Sprintf("伏笔「%s」已更新", fs.Name), nil
			},
		},
		{
			Name:        "delete_foreshadow",
			Description: "删除伏笔",
			Parameters:  `{"id": 1}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID int `json:"id"`
				}
				json.Unmarshal(args, &params)
				for i, fs := range ctx.State.Foreshadows {
					if fs.ID == params.ID {
						ctx.State.Foreshadows = append(ctx.State.Foreshadows[:i], ctx.State.Foreshadows[i+1:]...)
						if err := SaveProgress(ctx.ProgressPath, ctx.State); err != nil {
							return "", fmt.Errorf("保存失败: %w", err)
						}
						_ = SaveForeshadowRoadmap(filepath.Dir(ctx.ProgressPath), ctx.State)
						return fmt.Sprintf("伏笔「%s」已删除", fs.Name), nil
					}
				}
				return fmt.Sprintf("伏笔 %d 不存在", params.ID), nil
			},
		},
		{
			Name:        "read_skills",
			Description: "获取所有技能及启用状态",
			Parameters:  `{}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var result strings.Builder
				for _, s := range ctx.Skills {
					enabled := false
					if ctx.Config.SkillConfig != nil && ctx.Config.SkillConfig.EnabledSkills != nil {
						enabled = ctx.Config.SkillConfig.EnabledSkills[s.ID]
					}
					status := "❌"
					if enabled {
						status = "✅"
					}
					result.WriteString(fmt.Sprintf("%s [%s] %s (%s)\n  %s\n\n", status, s.Category, s.Name, s.ID, s.Description))
				}
				return result.String(), nil
			},
		},
		{
			Name:        "toggle_skill",
			Description: "启用或禁用技能",
			Parameters:  `{"id": "技能ID", "enabled": true}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				var params struct {
					ID      string `json:"id"`
					Enabled bool   `json:"enabled"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}
				found := false
				for _, s := range ctx.Skills {
					if s.ID == params.ID {
						found = true
						break
					}
				}
				if !found {
					return "", fmt.Errorf("技能不存在: %s", params.ID)
				}
				if ctx.Config.SkillConfig == nil {
					ctx.Config.SkillConfig = &SkillConfig{EnabledSkills: make(map[string]bool)}
				}
				if ctx.Config.SkillConfig.EnabledSkills == nil {
					ctx.Config.SkillConfig.EnabledSkills = make(map[string]bool)
				}
				ctx.Config.SkillConfig.EnabledSkills[params.ID] = params.Enabled
				if err := saveConfig(ctx.CfgPath, ctx.Config); err != nil {
					return "", fmt.Errorf("保存配置失败: %w", err)
				}
				status := "禁用"
				if params.Enabled {
					status = "启用"
				}
				return fmt.Sprintf("技能「%s」已%s", params.ID, status), nil
			},
		},
		{
			Name:        "reset_progress",
			Description: "【危险·不可逆】重置所有进度，清除全部章节、大纲和伏笔。仅当用户明确要求重置/清空整个项目进度时使用，且必须先向用户确认。",
			Parameters:  `{"confirm": true}`,
			Execute: func(args json.RawMessage, ctx *AgentContext) (string, error) {
				if msg := requireConfirm(args, fmt.Sprintf("重置全部进度（共 %d 章及所有伏笔）", len(ctx.State.Chapters))); msg != "" {
					return msg, nil
				}
				if err := deleteFile(ctx.ProgressPath); err != nil {
					return "", fmt.Errorf("删除进度文件失败: %w", err)
				}
				// 原地清空，保证 Handlers 持有的同一指针也被重置
				*ctx.State = Progress{Phase: "outline"}
				ctx.Logger.Success("进度已重置。")
				return "进度已重置。", nil
			},
		},
	}
}
