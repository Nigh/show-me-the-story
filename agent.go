package main

import (
	"encoding/json"
	"fmt"
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
}

type AgentStep struct {
	Role      string
	Content   string
	ToolCall  *ToolCall
	ToolResult string
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func RunAgentLoop(ctx *AgentContext, userMessage string, history []AgentStep, maxSteps int) (string, []AgentStep, error) {
	tools := getBuiltinTools()
	toolDesc := buildToolDescriptions(tools)

	systemPrompt := buildAgentSystemPrompt(ctx, toolDesc)

	var messages []Message
	messages = append(messages, Message{Role: "system", Content: systemPrompt})

	for _, step := range history {
		if step.Role == "assistant" {
			if step.ToolCall != nil {
				tcJSON, _ := json.Marshal(step.ToolCall)
				messages = append(messages, Message{Role: "assistant", Content: fmt.Sprintf("<tool_call>\n%s\n</tool_call>", string(tcJSON))})
			} else {
				messages = append(messages, Message{Role: "assistant", Content: step.Content})
			}
		} else if step.Role == "tool" {
			messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("[工具结果]\n%s", step.ToolResult)})
		}
	}

	messages = append(messages, Message{Role: "user", Content: userMessage})

	for step := 0; step < maxSteps; step++ {
		fullResp := ""
		err := callAgentAPI(ctx.APICfg, messages, func(chunk string) {
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
		messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("[工具结果]\n%s", result)})
	}

	return "已达到最大工具调用步骤限制。", history, nil
}

func callAgentAPI(apiCfg *APIConfig, messages []Message, onChunk func(string)) error {
	_, err := CallAPIStream(apiCfg, messages[0].Content, formatMessages(messages[1:]), onChunk)
	if err != nil {
		result, err2 := CallAPI(apiCfg, messages[0].Content, formatMessages(messages[1:]))
		if err2 != nil {
			return err
		}
		if onChunk != nil {
			onChunk(result)
		}
	}
	return nil
}

func formatMessages(msgs []Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			sb.WriteString(fmt.Sprintf("[系统] %s\n\n", m.Content))
		} else if m.Role == "assistant" {
			sb.WriteString(fmt.Sprintf("[助手] %s\n\n", m.Content))
		} else {
			sb.WriteString(fmt.Sprintf("[用户] %s\n\n", m.Content))
		}
	}
	return sb.String()
}

func buildAgentSystemPrompt(ctx *AgentContext, toolDesc string) string {
	var sb strings.Builder
	sb.WriteString("你是一个小说创作助手，可以帮助作者管理角色、世界观、查看大纲和章节内容。\n\n")

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
	sb.WriteString("当需要调用工具时，使用以下格式（必须是合法的JSON）：\n")
	sb.WriteString("<tool_call>\n")
	sb.WriteString(`{"name": "工具名称", "arguments": {参数}}`)
	sb.WriteString("\n</tool_call>\n\n")
	sb.WriteString("一次只能调用一个工具。等收到工具结果后再继续。\n")
	sb.WriteString("当不需要调用工具时，直接回复用户即可。\n")

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
		return parseToolCallJSON(content)
	}

	endIdx := strings.Index(content[idx:], "</tool_call>")
	if endIdx == -1 {
		return parseToolCallJSON(content)
	}

	jsonStr := strings.TrimSpace(content[idx+len("<tool_call>") : idx+endIdx])
	return parseToolCallFromJSON(jsonStr)
}

func parseToolCallJSON(content string) *ToolCall {
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil
	}
	return parseToolCallFromJSON(jsonStr)
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
					result.WriteString(fmt.Sprintf("【%s】(%s)\n  %s\n", o.Name, o.Type, o.Description))
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
					result.WriteString(fmt.Sprintf("#%d [%s] %s\n  %s\n\n", fs.ID, fs.Status, fs.Name, fs.Description))
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

						return fmt.Sprintf("角色「%s」已更新", ctx.Settings.Characters[i].Name), nil
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

						return fmt.Sprintf("世界观条目「%s」已更新", ctx.Settings.Worldview[i].Name), nil
					}
				}
				return fmt.Sprintf("未找到世界观条目: %s", params.ID), nil
			},
		},
	}
}
