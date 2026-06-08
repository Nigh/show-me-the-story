# AGENTS.md — AI 小说生成器项目指南

> **重要**：当对项目进行任何修改（代码、配置、前端、提示词等）后，必须同步更新本文件，确保文档与项目实际情况完全一致。

## 项目概述

单二进制 Go Web 应用，零外部依赖（仅标准库），通过 OpenAI 兼容 API 自动生成长篇小说。前端为单文件 `static/index.html`（vanilla JS，无框架），通过 `embed.FS` 内嵌到二进制中。

- **Go 版本**：1.25.1
- **模块名**：`showmethestory`
- **默认端口**：`:8090`（可通过 `PORT` 环境变量覆盖）

## 编译与运行

```bash
go build -o show-me-the-story.exe .   # 编译
./show-me-the-story.exe               # 运行（默认当前目录为项目目录）
./show-me-the-story.exe ./my-novel/   # 指定项目目录运行
```

编译前务必确认 `go build` 无报错。项目无测试框架，编译通过即为基本验证。

## 架构概览

```
用户浏览器 ←→ HTTP Server (web.go)
                  │
                  ├─ handlers.go    ← 所有 API 端点处理（含设定 CRUD、技能、聊天）
                  │   ├─ 同步端点：直接返回 JSON
                  │   └─ 异步端点：tryStartTask() → go func() { ... endTask() } → SSE 推送
                  │
                  ├─ SSE (logger.go) ← 实时日志/进度/事件推送到前端
                  │
                  ├─ outline.go     ← 大纲阶段逻辑 + EditChapterOutline
                  ├─ writing.go     ← 写作阶段逻辑 + 上下文注入 + 去AI味
                  ├─ foreshadow.go  ← 伏笔系统
                  ├─ continue.go    ← 续写功能（导入分析）
                  ├─ reconcile.go   ← 设定协调逻辑（AI 自动兼容新旧设定）
                  ├─ settings.go    ← 结构化设定（角色/世界观/组织/关系）CRUD + 持久化
                  ├─ skills.go      ← Skill 系统（内置 + 项目级，可选启用）
                  ├─ agent.go       ← Agent Loop 引擎 + 内置工具集（全局助理用）
                  ├─ chat.go        ← 会话管理（JSON 文件存储）
                  ├─ api.go         ← OpenAI API 调用 + 重试
                  ├─ config.go      ← 配置结构体 + 加载/保存（含 SkillConfig）
                  ├─ state.go       ← 进度/章节/伏笔结构体 + 持久化
                  ├─ prompts.go     ← 提示词模板渲染 + 内置默认模板
                  └─ filesys.go     ← 文件操作抽象
```

## 文件清单与职责

| 文件 | 职责 |
|------|------|
| `main.go` | 入口，接受命令行项目目录参数，加载API配置/故事配置/进度/设定/技能，创建 sessions 目录，启动 Web 服务器 |
| `config.go` | `APIConfig`、`Config`（含 `SkillConfig`）、`StoryConfig`、`PromptsConfig` 结构体，Load/Save 函数，`applyDefaults` |
| `state.go` | `Progress`、`ChapterState`、`Foreshadow` 结构体，`LoadProgress`、`SaveProgress`、`SaveChapterMarkdown` |
| `api.go` | `CallAPI`（同步）、`CallAPIStream`（流式）、`CallAPIWithRetry`/`CallAPIWithRetryLog`（无限重试）、`CallAPIStreamWithRetry`/`CallAPIStreamWithRetryLog` |
| `outline.go` | `generateOutline`、`reviseOutline`、`GenerateOutlineAction`、`ReviseOutlineAction`、`ConfirmOutlineAction`、`EditChapterOutline`、`cleanJSONResponse` |
| `writing.go` | `GenerateChapterAction`、`ReviseChapterAction`、`ConfirmChapterAction`、`PolishChapterAction`、章节内容生成/摘要/事实核查/流式输出、`buildHistorySummary`、`buildCharacterContext`、`buildWorldviewContext` |
| `foreshadow.go` | `SuggestForeshadows`、`UpdateForeshadows`、伏笔格式化注入、伏笔告警、`NextForeshadowID` |
| `continue.go` | `AnalyzeExistingContent`、`ImportContinueAction`、`GenerateContinuationOutline`、`splitContentByChapters` |
| `reconcile.go` | `ReconcileSettingsAction`、`regeneratePendingOutlines`、设定协调逻辑 |
| `settings.go` | `Character`、`WorldviewEntry`、`Organization`、`Relation`、`ProjectSettings` 结构体，`LoadProjectSettings`、`SaveProjectSettings`、`buildCharacterContext`、`buildWorldviewContext` |
| `skills.go` | `Skill`、`SkillConfig` 结构体，`LoadBuiltinSkills`、`LoadProjectSkills`、`MergeSkills`、`GetEnabledSkills`、`GetEnabledSkillsByCategory`、`FormatSkillsContent`，`//go:embed embeds/skills` |
| `agent.go` | `Tool`、`AgentContext`、`AgentStep`、`ToolCall` 结构体，`RunAgentLoop`、工具调用解析、内置工具集（读/写角色/世界观/章节等） |
| `chat.go` | `ChatSession`、`ChatMessage`、`ChatSessionIndex` 结构体，`LoadChatSessions`、`LoadChatSession`、`SaveChatSession`、`DeleteChatSession` |
| `handlers.go` | 所有 HTTP handler（含设定 CRUD、技能 toggle、聊天、去AI味）、`tryStartTask`/`endTask` 互斥、`writeJSON`/`writeError`、`writeFileAtomic` |
| `web.go` | 路由注册、CORS/日志中间件、静态文件服务、`startWebServer` |
| `logger.go` | `LogBroadcaster`（SSE 广播）、所有日志/事件方法（含 `ChatChunk`、`ToolCallStart`、`ToolCallEnd`、`PolishResult`） |
| `prompts.go` | `RenderPrompt`（`{{.KeyName}}` 替换）、`DefaultPrompts` 变量（所有内置提示词模板） |
| `filesys.go` | `writeFileImpl`、`deleteFileImpl`、`renameFileImpl` |
| `static/index.html` | 完整前端（HTML + CSS + JS），单文件，vanilla JS |
| `embeds/skills/*.md` | 内置 Skill 文件（YAML frontmatter + prompt body），通过 `//go:embed` 嵌入 |

## 关键设计模式

### 项目目录化

`main.go` 接受命令行参数 `os.Args[1]` 作为项目目录，默认为当前目录。所有文件路径（`progress.json`、`config.json`、`settings.json`、`sessions/`）都相对于项目目录。`api.json` 优先查找项目目录，fallback 到程序同目录。

### 异步任务模式

所有 AI 调用的 handler 都遵循此模式：

```go
func (h *Handlers) PostXxxAction(w http.ResponseWriter, r *http.Request) {
    if !h.tryStartTask() {                    // 互斥：同一时间只能有一个 AI 任务
        h.writeError(w, http.StatusConflict, "有任务正在运行")
        return
    }
    go func() {
        h.logger.TaskStart("task_name")       // SSE: task_start 事件
        // ... 调用 AI ...
        h.endTask()                           // 释放锁
        h.logger.TaskEnd("task_name", true)   // SSE: task_end 事件
        h.broadcastProgress()                 // SSE: progress_update 事件
    }()
    h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
```

### 提示词渲染

使用简单的 `strings.ReplaceAll`，不是 Go `text/template`：

```go
userPrompt := RenderPrompt(cfg.Prompts.ChapterWriting, map[string]string{
    "Title":      state.Title,
    "ChapterNum": fmt.Sprintf("%d", ch.Num),
    // ...
})
```

模板中用 `{{.KeyName}}` 作为占位符。新增 prompt 变量必须遵循此约定。

### 双配置结构 + SkillConfig

API 配置（`APIConfig`）与故事配置（`Config`）完全分离，分别保存为 `api.json` 和 `config.json`。`Config` 中包含 `SkillConfig` 字段，存储技能启用状态。所有 AI 调用函数接收 `*APIConfig`，故事相关函数同时接收 `*APIConfig` 和 `*Config`。

### Skill 可选性设计

所有 skill 默认 `enabled: false`，配置存储在 `config.json` 的 `skill_config` 中。功能性 AI（大纲/章节/核查）默认不注入任何 skill。作者在前端 Skill 管理页手动 toggle 启用。

注入规则：
- 大纲生成/章节写作/修订/事实核查/AI设定生成：不注入任何 skill（除非作者显式启用）
- 去AI味按钮：加载所有 enabled 的 `polish` 类 skill
- 全局助理：加载所有 enabled 的 skill 作为参考

### Agent Loop

独立 `agent.go` 文件，`RunAgentLoop` 函数实现工具调用循环。内置工具集包括：`read_characters`、`read_character`、`read_worldview`、`read_organizations`、`read_chapter`、`read_outline`、`read_foreshadows`、`search_project`、`create_character`、`update_character`、`create_worldview`、`update_worldview`。仅全局助理使用 Agent Loop。

工具调用解析支持：`<tool_call>` XML 标签、JSON 代码块、裸 JSON 对象（含 `name`/`tool` 键）。

### API 调用重试

`CallAPIWithRetry` / `CallAPIStreamWithRetry` 为无限重试 + 指数退避（最大 30s）。带 `Log` 后缀的变体通过 SSE 推送重试信息。

### 流式输出

`CallAPIStream` 返回流式响应，通过 `onChunk` 回调实时推送每个 token。`ContentChunk` SSE 事件用于前端实时渲染，`StreamProgress` 事件用于日志面板显示字符数进度（每 500 字触发一次）。

### 章节状态机

```
pending → writing → review → accepted
                           ↗
                    （修改后回到 review）
```

### 伏笔生命周期

```
planted → progressing → resolved
                     → abandoned
```

### 进度持久化

每个关键步骤后立即保存 `progress.json`。API 配置保存 `api.json`，故事配置保存 `config.json`。设定保存 `settings.json`。使用原子写入（先写 `.tmp` 再 rename）。

### 结构化设定

`settings.json` 存储 `ProjectSettings`，包含 `Characters`、`Worldview`、`Organizations`、`Relations` 四个数组。这些设定在章节写作时通过 `buildCharacterContext`/`buildWorldviewContext` 注入到 prompt 中。设定也支持 AI 自动生成（`POST /api/settings/ai-generate`）。

### 会话管理

聊天会话存储为项目目录 `sessions/` 下的 JSON 文件。`sessions/index.json` 为会话索引。每个会话文件名为 `{id}.json`。使用 `writeFileAtomic` 保持一致性。

## 续写功能流程

```
大纲页空状态 → 点击"导入已有内容" → 展开 textarea
  → 粘贴已有文本 → POST /api/continue/import (异步分析)
  → SSE continue_analysis 事件返回 ContinueAnalysis
  → 大纲页展示可编辑的元数据 + 章节大纲/摘要
  → 用户编辑后点击"确认导入" → POST /api/continue/confirm
  → ImportContinueAction：设置 Phase="outline"，已有章节 status=accepted
  → 大纲页显示已导入的 accepted 章节 + "生成后续大纲"按钮
  → POST /api/outline/generate-continuation (异步)
  → 追加续写章节为 pending
  → 确认大纲 → 进入写作阶段
```

## 设定协调流程

```
配置页修改设定 → 保存故事配置 (PUT /api/config)
  → 若存在已确认章节 → POST /api/settings/reconcile (异步)
  → ReconcileSettingsAction：
    1. 收集 accepted 章节摘要
    2. AI 比对新设定 vs 已有内容，输出兼容设定
    3. 更新 cfg.Story + state.StoryConfigSnapshot
    4. 若存在 pending 章节，基于新设定重新生成其大纲
    5. 原子保存 config.json + progress.json
    6. SSE 推送 settings_reconciled 事件
  → 前端收到事件后重新加载 config/progress，显示协调结果
```

## API 端点一览

| 方法 | 路径 | 同步/异步 | 说明 |
|------|------|----------|------|
| GET | `/api/config/api` | 同步 | 获取 API 配置 |
| PUT | `/api/config/api` | 同步 | 保存 API 配置 |
| GET | `/api/config` | 同步 | 获取故事配置 |
| PUT | `/api/config` | 同步 | 保存故事配置 |
| GET | `/api/progress` | 同步 | 获取进度 |
| DELETE | `/api/progress` | 同步 | 重置进度 |
| GET | `/api/status` | 同步 | 获取状态摘要 |
| POST | `/api/outline/generate` | 异步 | 生成大纲 |
| POST | `/api/outline/confirm` | 同步 | 确认大纲 |
| POST | `/api/outline/revise` | 异步 | 修订大纲 |
| POST | `/api/outline/generate-continuation` | 异步 | 生成续写大纲 |
| PUT | `/api/outline/{num}` | 同步 | 编辑指定 pending 章节大纲 |
| POST | `/api/settings/reconcile` | 异步 | 协调设定与已有内容 |
| GET | `/api/settings` | 同步 | 获取结构化设定（角色/世界观/组织/关系） |
| POST | `/api/settings/ai-generate` | 异步 | AI 自动生成初始设定 |
| POST | `/api/characters` | 同步 | 创建角色 |
| PUT | `/api/characters/{id}` | 同步 | 更新角色 |
| DELETE | `/api/characters/{id}` | 同步 | 删除角色 |
| POST | `/api/worldview` | 同步 | 创建世界观条目 |
| PUT | `/api/worldview/{id}` | 同步 | 更新世界观条目 |
| DELETE | `/api/worldview/{id}` | 同步 | 删除世界观条目 |
| POST | `/api/organizations` | 同步 | 创建组织 |
| PUT | `/api/organizations/{id}` | 同步 | 更新组织 |
| DELETE | `/api/organizations/{id}` | 同步 | 删除组织 |
| POST | `/api/relations` | 同步 | 创建关系 |
| PUT | `/api/relations/{id}` | 同步 | 更新关系 |
| DELETE | `/api/relations/{id}` | 同步 | 删除关系 |
| POST | `/api/chapter/generate` | 异步 | 生成章节 |
| POST | `/api/chapter/confirm` | 同步 | 确认章节 |
| POST | `/api/chapter/revise` | 异步 | 修订章节 |
| POST | `/api/chapter/polish` | 异步 | 去AI味（需启用 polish 类技能） |
| DELETE | `/api/chapter` | 同步 | 删除最后章节 |
| DELETE | `/api/chapters/from/{num}` | 同步 | 从第 N 章删除到末尾 |
| DELETE | `/api/outline` | 同步 | 删除大纲 |
| GET | `/api/foreshadows` | 同步 | 获取伏笔列表 |
| POST | `/api/foreshadows/suggest` | 异步 | AI 建议伏笔 |
| POST | `/api/foreshadows/confirm` | 同步 | 批量确认伏笔 |
| POST | `/api/foreshadows` | 同步 | 手动创建伏笔 |
| PUT | `/api/foreshadows/{id}` | 同步 | 更新伏笔 |
| DELETE | `/api/foreshadows/{id}` | 同步 | 删除伏笔 |
| POST | `/api/continue/import` | 异步 | 分析已有内容 |
| POST | `/api/continue/confirm` | 同步 | 确认续写导入 |
| GET | `/api/skills` | 同步 | 获取所有技能及启用状态 |
| PUT | `/api/skills/{id}/toggle` | 同步 | 切换技能启用/禁用 |
| GET | `/api/chat/sessions` | 同步 | 获取会话列表 |
| POST | `/api/chat/sessions` | 同步 | 创建新会话 |
| GET | `/api/chat/sessions/{id}` | 同步 | 获取会话详情（含消息） |
| DELETE | `/api/chat/sessions/{id}` | 同步 | 删除会话 |
| POST | `/api/chat/sessions/{id}/messages` | 异步 | 发送消息（Agent Loop，SSE 流式返回） |
| GET | `/api/events` | SSE | 实时事件流 |

## SSE 事件类型

| 事件 | 数据 | 触发时机 |
|------|------|---------|
| `log` | `{level, msg, time}` | 所有日志消息 |
| `task_start` | `{task}` | 异步任务开始 |
| `task_end` | `{task, success}` | 异步任务结束 |
| `progress_update` | `{phase, title, current_chapter, total_chapters, ...}` | 进度变化 |
| `content_chunk` | `{chapter_idx, text}` | 流式生成 token |
| `stream_progress` | `{chapter_idx, char_count}` | 流式生成字符数进度（每 500 字） |
| `foreshadow_suggestions` | `ForeshadowSuggestion[]` | 伏笔建议结果 |
| `continue_analysis` | `ContinueAnalysis` | 续写分析结果 |
| `settings_reconciled` | `{explanation, changed_fields}` | 设定协调完成 |
| `chat_chunk` | `{session_id, text}` | 助理流式回复 |
| `tool_call_start` | `{session_id, tool_name, args}` | Agent 工具调用开始 |
| `tool_call_end` | `{session_id, tool_name, result}` | Agent 工具调用结束 |
| `polish_result` | `{chapter_idx, text}` | 去AI味结果 |

## PromptsConfig 字段

| 字段 | JSON key | 用途 |
|------|----------|------|
| `OutlineGeneration` | `outline_generation` | 大纲生成 |
| `ChapterWriting` | `chapter_writing` | 章节创作 |
| `ChapterSummary` | `chapter_summary` | 摘要提炼 |
| `FactCheck` | `fact_check` | 事实核查 |
| `OutlineRevision` | `outline_revision` | 大纲修订 |
| `ForeshadowPlanning` | `foreshadow_planning` | 伏笔规划 |
| `ForeshadowUpdate` | `foreshadow_update` | 伏笔状态更新 |
| `ContentAnalysis` | `content_analysis` | 续写内容分析 |
| `ContinuationOutlineGeneration` | `continuation_outline_generation` | 续写大纲生成 |
| `SettingsReconciliation` | `settings_reconciliation` | 设定协调 |

新增 prompt 模板时需要：(1) 在 `PromptsConfig` 添加字段，(2) 在 `DefaultPrompts` 添加默认值，(3) 在 `applyDefaults` 添加 fallback。

## ChapterWriting 模板占位符

| 占位符 | 来源 | 说明 |
|--------|------|------|
| `{{.Title}}` | `state.Title` | 小说标题 |
| `{{.ChapterNum}}` | `ch.Num` | 章节编号 |
| `{{.CorePrompt}}` | `state.CorePrompt` | 核心写作提示词 |
| `{{.CoreRequirements}}` | `state.CoreRequirements` | 核心写作要求 |
| `{{.HistorySummary}}` | `buildHistorySummary()` | 最近 5 章摘要 |
| `{{.ChapterTitle}}` | `ch.Title` | 本章标题 |
| `{{.ChapterOutline}}` | `ch.Outline` | 本章大纲 |
| `{{.WritingStyle}}` | snapshot | 写作风格 |
| `{{.CharacterSetting}}` | snapshot | 原始角色设定文本 |
| `{{.CharacterContext}}` | `buildCharacterContext()` | 结构化角色详情（从 settings 匹配） |
| `{{.WorldSetting}}` | snapshot | 原始世界观设定文本 |
| `{{.WorldviewContext}}` | `buildWorldviewContext()` | 结构化世界观详情（从 settings 匹配） |
| `{{.TargetWords}}` | snapshot | 每章目标字数 |
| `{{.Foreshadows}}` | `formatActiveForeshadowsForChapter()` | 活跃伏笔上下文 |

## 内置 Skill 文件

| 文件 | ID | 分类 | 说明 |
|------|----|------|------|
| `embeds/skills/humanizer-zh.md` | `humanizer-zh` | polish | 23 条禁止模式 + 高频短语替换表（top 50） + 口语化/格式规范规则 |
| `embeds/skills/story-deslop.md` | `story-deslop` | polish | 6-Gate 检测流程 + AI 味检测报告模板 + 真人写作基准表 |
| `embeds/skills/writing-craft.md` | `writing-craft` | writing | 章首钩子 7 式 + 章尾钩子 13 式 + 爽点密度 + 节奏控制 |

Skill 文件格式：YAML frontmatter（`---` 分隔）+ Markdown body。前端通过 `GET /api/skills` 获取列表，`PUT /api/skills/{id}/toggle` 切换启用状态。

## 前端架构

`static/index.html` 是单文件前端，包含 HTML + CSS + vanilla JS。

- **页面**：`config`（配置 + 角色管理 + 世界观管理）、`outline`（大纲，含导入功能）、`writing`（写作 + 去AI味按钮）、`relations`（关系图谱 Canvas）、`assistant`（全局助理聊天）、`skills`（技能管理）
- **导航**：`<nav>` 中 `<a data-page="xxx">` + hash 路由
- **SSE**：`EventSource` 连接 `/api/events`，监听各事件类型
- **全局对象**：`App` 包含所有状态和方法
- **全局函数**：每个 `App.xxx()` 方法有对应的全局函数包装（供 `onclick` 使用）
- **关系图谱**：`ForceGraph` 类，纯 Canvas 力导向布局，支持拖拽节点、悬浮 tooltip
- **聊天**：会话列表 + 消息区 + 工具调用卡片展示

## 重要约束

1. **零外部依赖**：仅使用 Go 标准库，不要引入第三方包
2. **单文件前端**：所有 HTML/CSS/JS 在 `static/index.html` 中，不要拆分
3. **嵌入式文件**：前端通过 `//go:embed static` 嵌入，skill 文件通过 `//go:embed embeds/skills` 嵌入，修改后需重新编译
4. **配置文件 gitignore**：`*.json` 被 gitignore，不要提交配置/进度/设定/会话文件
5. **提示词用 `{{.KeyName}}`**：不是 Go `text/template`，是简单字符串替换
6. **异步任务互斥**：同一时间只能有一个 AI 任务运行（`tryStartTask`/`endTask`）
7. **原子写入**：配置和进度文件使用 `writeFileAtomic`（先写 `.tmp` 再 rename）
8. **中文界面**：所有用户可见文本使用中文
9. **Skill 可选性**：所有 skill 默认禁用，功能性 AI 不注入任何 skill，除非作者显式启用

## 修改检查清单

完成代码修改后，必须执行以下检查：

1. `go build -o show-me-the-story.exe .` 编译通过
2. 确认无未使用的 import 或变量
3. 如果修改了 API 端点，确认 `web.go` 中路由已注册
4. 如果新增了 prompt 模板，确认 `config.go` 和 `applyDefaults` 已更新
5. 如果修改了 SSE 事件，确认前端已添加对应监听
6. 如果新增了 Skill 文件，确认在 `embeds/skills/` 目录中
7. **同步更新本 AGENTS.md 文件**
