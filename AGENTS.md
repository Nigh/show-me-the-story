# AGENTS.md — AI 小说生成器项目指南

> **重要**：当对项目进行任何修改（代码、配置、前端、提示词等）后，必须同步更新本文件，确保文档与项目实际情况完全一致。

## 项目概述

单二进制 Go Web 应用，Go 后端零外部依赖（仅标准库），通过 OpenAI 兼容 API 自动生成长篇小说。前端使用 Vite + Svelte 4 + DaisyUI 4 构建，产物通过 `embed.FS` 内嵌到二进制中。

- **Go 版本**：1.25.1
- **模块名**：`showmethestory`
- **默认端口**：`:48090`（可通过 `PORT` 环境变量覆盖）
- **前端**：Vite 5 + Svelte 4 + Tailwind CSS 4 + DaisyUI 5（xianii 暗色主题）
- **项目目录**：`storys/`（程序同目录下，每个故事项目一个子目录）

## 编译与运行

```bash
# 完整编译（含前端构建）
task build                          # 推荐：自动 npm run build + go build

# 或手动分步
cd frontend && npm install && npm run build && cd ..   # 构建前端
go build -o show-me-the-story.exe .                    # 编译 Go（嵌入 frontend/dist/）

# 运行
./show-me-the-story.exe               # 运行（默认当前目录为项目目录）
./show-me-the-story.exe ./my-novel/   # 指定项目目录运行

# 开发模式
task dev:frontend                     # 启动 Vite dev server（热重载，端口 5173，代理 /api → :48090）
task dev                              # 编译并启动 Go 后端
```

编译前务必确认 `go build` 无报错。项目无测试框架，编译通过即为基本验证。

## 架构概览

```
用户浏览器 ←→ HTTP Server (web.go)
                  │
                  ├─ handlers.go    ← 所有 API 端点处理（含设定 CRUD、技能、聊天、去AI味）
│   ├─ 同步端点：直接返回 JSON
│   └─ 异步端点：tryStartTask() → go func() { defer h.endTask(); ... } → SSE 推送
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
                  ├─ api.go         ← OpenAI API 调用 + 重试 + 致命错误检测 + context 支持
                  ├─ config.go      ← 配置结构体 + 加载/保存（含 SkillConfig）
                  ├─ state.go       ← 进度/章节/伏笔结构体 + 持久化
                  ├─ prompts.go     ← 提示词模板渲染 + 内置默认模板
                  └─ filesys.go     ← 文件操作抽象
```

## 文件清单与职责

| 文件 | 职责 |
|------|------|
| `main.go` | 入口，确定程序目录（`progDir`），创建 `storys/` 目录，加载 API 配置，启动 Web 服务器（无项目选择状态） |
| `config.go` | `APIConfig`、`Config`（含 `SkillConfig`）、`StoryConfig`、`PromptsConfig` 结构体，Load/Save 函数，`applyDefaults` |
| `state.go` | `Progress`、`ChapterState`、`Foreshadow` 结构体，`LoadProgress`、`SaveProgress`（原子写入）、`ChapterMarkdownPath`、`SaveChapterMarkdown(projectDir, ...)`（写入项目目录） |
| `api.go` | `CallAPI`/`CallAPIMessages`（同步）、`CallAPIStream`/`CallAPIStreamMessages`（流式，支持完整多轮消息历史）、`CallAPIWithRetry`/`CallAPIWithRetryLog`（无限重试）、`CallAPIStreamWithRetry`/`CallAPIStreamWithRetryLog`，`validateAPIConfig`、`isFatalAPIError`（401/403/404 致命，网络超时可重试） |
| `outline.go` | `generateOutline`、`reviseOutline`、`GenerateOutlineAction`（存在已确认章节时拒绝整体重新生成）、`ReviseOutlineAction`、`ConfirmOutlineAction`、`EditChapterOutline`、`cleanJSONResponse` |
| `writing.go` | `GenerateChapterAction`、`ReviseChapterAction`（当前审核中章节）、`ReviseSpecificChapterAction`（定向最小化修订任意章节，不影响其他章节）、`ConfirmChapterAction`、`PolishChapterAction`、`parseFactCheckResult`（JSON 优先 + 字符串 fallback）、章节内容生成/摘要/事实核查/流式输出、`buildHistorySummary` |
| `foreshadow.go` | `SuggestForeshadows`、`UpdateForeshadows`、伏笔格式化注入、伏笔告警、`NextForeshadowID` |
| `continue.go` | `AnalyzeExistingContent`、`ImportContinueAction`、`GenerateContinuationOutline`、`splitContentByChapters` |
| `reconcile.go` | `ReconcileSettingsAction`、`regeneratePendingOutlines`、设定协调逻辑 |
| `settings.go` | `Character`、`WorldviewEntry`、`Organization`、`Relation`、`ProjectSettings` 结构体，`LoadProjectSettings`、`SaveProjectSettings`、`buildCharacterContext`、`buildWorldviewContext` |
| `skills.go` | `Skill`、`SkillConfig` 结构体，`LoadBuiltinSkills`、`LoadProjectSkills`、`MergeSkills`、`GetEnabledSkills`、`GetEnabledSkillsByCategory`、`FormatSkillsContent`，`//go:embed embeds/skills` |
| `agent.go` | `Tool`、`AgentContext`、`AgentStep`、`ToolCall` 结构体，`RunAgentLoop`（多轮消息历史）、工具调用解析、内置工具集（读/写角色/世界观/章节等）、系统提示词含安全规则与工具选择指南、`requireConfirm`（破坏性工具需 `confirm: true`） |
| `chat.go` | `ChatSession`、`ChatMessage`、`ChatSessionIndex` 结构体，`LoadChatSessions`、`LoadChatSession`、`SaveChatSession`、`DeleteChatSession` |
| `handlers.go` | `Handlers` 结构体（含项目管理字段 `progDir`/`projectName`/`projectMu`）、`projectDir()` 帮助函数、项目切换 `switchProject()`、`ensureProject()` 检查、所有 HTTP handler（含 `PostChapterReviseSpecific` 定向修订）、`tryStartTask`/`endTask`/`startChildWork` 互斥、项目管理 handler（`GetProjects`/`PostProject`/`PostProjectSelect`/`GetProjectCurrent`/`DeleteProject`） |
| `web.go` | 路由注册（含项目管理端点）、CORS/日志中间件、静态文件服务、`startWebServer`、项目管理 handler（`GetProjects`/`PostProject`/`GetProjectCurrent`/`PostProjectSelect`/`DeleteProject`） |
| `logger.go` | `LogBroadcaster`（SSE 广播）、所有日志/事件方法（含 `ChatChunk`、`ToolCallStart`、`ToolCallEnd`、`PolishResult`） |
| `prompts.go` | `RenderPrompt`（`{{.KeyName}}` 替换）、`DefaultPrompts` 变量（所有内置提示词模板） |
| `filesys.go` | `writeFileImpl`、`deleteFileImpl`、`renameFileImpl` |
| `embeds/skills/*.md` | 内置 Skill 文件（YAML frontmatter + prompt body），通过 `//go:embed` 嵌入 |

### 前端文件（`frontend/`）

| 文件 | 职责 |
|------|------|
| `package.json` | 前端依赖：Svelte 4、Vite 5、Tailwind CSS 4、DaisyUI 5 |
| `vite.config.js` | Vite 配置：`@tailwindcss/vite` 插件、Svelte 插件、dev server 代理 `/api` → `:48090`、构建输出到 `dist/` |
| `svelte.config.js` | Svelte 预处理器配置 |
| `index.html` | 入口 HTML，`data-theme="xianii"` |
| `src/main.js` | Svelte 应用挂载点 |
| `src/app.css` | 全局样式：Tailwind 指令 + 自定义滚动条/toast 动画 |
| `src/App.svelte` | 根组件：Header（阶段badge + 章节进度badge + AI思考中badge） + 顶部导航（带图标） + 页面路由 + LogPanel + Toast 容器 |
| `src/lib/api.js` | `api(method, url, body)` — fetch 封装 |
| `src/lib/router.js` | `currentPage` store + hash 路由监听 |
| `src/lib/stores.js` | 全局 Svelte stores（progress、config、settings、taskRunning、lastFailedTask 等）+ toast/log 管理 |
| `src/lib/sse.js` | `connectSSE()` — EventSource 连接 + 12 种事件处理 → 更新 stores |
| `src/pages/Config.svelte` | 配置页：API 配置、故事配置（直接 PUT 保存 + 关键设定变更时提示协调）、角色管理、世界观管理 |
| `src/pages/Outline.svelte` | 大纲页：直接操作按钮（生成/确认/修订意见/删除/生成后续大纲）+ 导入续写 + pending 章节内联编辑 + 流式预览 |
| `src/pages/Writing.svelte` | 写作页：章节列表（状态点）+ 直接操作（生成/确认/修改意见，自动区分当前章修订与定向修订）+ 导出 TXT + 复制 + 上下章导航 + 流式自动滚动 |
| `src/pages/Relations.svelte` | 图谱页：Canvas 力导向图谱（ForceGraph 类），支持拖拽 |
| `src/pages/Assistant.svelte` | 助理页：聊天会话列表 + 消息区 + 工具调用卡片 + 流式回复 |
| `src/pages/Skills.svelte` | 技能页：技能表格 + toggle 开关 |
| `src/components/ChatPanel.svelte` | 右侧聊天面板：会话列表 + 停止按钮 + 任务状态/日志区 + 消息区 + 工具调用卡片（中文工具名映射、危险工具红色高亮、区分 running/done）+ 流式回复 + 智能自动滚动 + 输入框自动增高 + 失败重试 banner |
| `src/components/ConfirmModal.svelte` | 全局确认弹窗组件（替代浏览器 confirm） |
| `src/components/LogPanel.svelte` | 底部可折叠实时日志面板 |

## 关键设计模式

### 项目目录化

`main.go` 接受命令行参数 `os.Args[1]` 作为程序基础目录（`progDir`），默认为当前目录。在 `progDir` 下自动创建 `storys/` 目录，每个故事项目是 `storys/{projectName}/` 子目录。`api.json` 始终在 `progDir` 下（全局共享）。所有项目文件（`progress.json`、`config.json`、`settings.json`、`sessions/`）都在各自项目目录中。

启动时不绑定具体项目，前端显示项目选择页面。用户选择/创建项目后，后端通过 `switchProject()` 加载对应项目的全部数据。

### 前端构建

前端使用 Vite + Svelte 构建，开发和构建流程：

```bash
task dev:frontend   # 启动 Vite dev server（端口 5173），热重载，代理 /api → :48090
task frontend:build # 构建前端产物到 frontend/dist/
task build          # 完整构建：frontend:build + go build
```

开发模式下，前端通过 Vite dev server 的 proxy 访问 Go 后端 API。生产构建时，`frontend/dist/` 通过 `//go:embed` 嵌入 Go 二进制。

### 异步任务模式

所有 AI 调用的 handler 都遵循此模式：

```go
func (h *Handlers) PostXxxAction(w http.ResponseWriter, r *http.Request) {
    if !h.tryStartTask() {                    // 互斥：同一时间只能有一个 AI 任务
        h.writeError(w, http.StatusConflict, "有任务正在运行")
        return
    }
    go func() {
        defer h.endTask()                     // defer 确保 TaskEnd 之后才释放锁
        h.logger.TaskStart("task_name")       // SSE: task_start 事件
        ctx := h.taskCtx                      // 捕获任务 context
        // ... 调用 AI（传入 ctx）...
        h.logger.TaskEnd("task_name", true)   // SSE: task_end 事件
        h.broadcastProgress()                 // SSE: progress_update 事件
    }()
    h.writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
```

`tryStartTask()` 创建带 cancel 的 `context.Context`，设置 `activeWork=1`。`endTask()` 递减 `activeWork`，仅当归零时才释放锁和 cancel context。`PostTaskStop` handler 调用 `taskCancel()` 取消运行中的任务。`startChildWork()` 用于 Agent 子任务（异步工具调用），增加 `activeWork` 计数但不创建新 context。`isTaskRunning()` 检查 `taskRunning || activeWork > 0`。

### 任务重入防护

- **后端**：`tryStartTask()` 检查 `taskRunning || activeWork > 0`，确保主任务和子任务期间都不会被新任务抢占
- **后端**：使用 `defer h.endTask()` 确保 `TaskEnd` 事件和 `broadcastProgress` 在锁释放前完成
- **前端**：所有按钮使用 `disabled={$taskRunning}` 禁用
- **前端**：发送消息前检查 `$taskRunning`，API 返回 409 时显示错误提示

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

独立 `agent.go` 文件，`RunAgentLoop(goCtx context.Context, ctx *AgentContext, userMessage string, history []AgentStep, maxSteps int)` 函数实现工具调用循环，接受 `context.Context` 支持任务取消。最大工具调用步骤为 30（安全上限，AI 自然终止不受此限）。内置工具集包括：`read_characters`、`read_character`、`read_worldview`、`read_organizations`、`read_chapter`、`read_outline`、`read_foreshadows`、`search_project`、`create_character`、`update_character`、`create_worldview`、`update_worldview`、`delete_character`、`delete_worldview`、`create_organization`、`update_organization`、`delete_organization`、`create_relation`、`update_relation`、`delete_relation`、`read_project_config`、`update_project_config`、`generate_outline`、`confirm_outline`、`revise_outline`、`delete_outline`、`edit_chapter_outline`、`generate_chapter`、`confirm_chapter`、`revise_chapter`、`delete_chapter`、`delete_chapters_from`、`suggest_foreshadows`、`create_foreshadow`、`update_foreshadow`、`delete_foreshadow`、`read_skills`、`toggle_skill`、`reset_progress`。仅全局助理使用 Agent Loop。

工具调用解析支持：`<tool_call>` XML 标签（含 JSON 或 XML 内容）、JSON 代码块、裸 JSON 对象（含 `name`/`tool` 键）。解析具有多级 fallback：`<tool_call>` 内 JSON → `<tool_call>` 内 XML 格式 → `</tool_call>` 之后的 JSON → 全文 JSON → `function.name()` 格式。`parseToolCallJSON` 遍历内容中所有 JSON 对象而非仅第一个。

### Agent 安全护栏

防止 AI 误删用户数据的多层防护：

1. **系统提示词安全规则**：`buildAgentSystemPrompt` 包含最高优先级的「安全规则」（修改 ≠ 删除）和「工具选择指南」，明确指示修改章节细节必须用 `revise_chapter` 而非删除重写
2. **破坏性工具二次确认**：`delete_chapter`、`delete_chapters_from`、`delete_outline`、`reset_progress` 必须传入 `confirm: true` 参数，否则返回警告信息要求 AI 先向用户确认
3. **`revise_chapter` 支持任意章节**：可选 `num` 参数，当前审核中章节走 `ReviseChapterAction`（完整流程），其他章节（含已确认）走 `ReviseSpecificChapterAction`（最小化定向修订，不影响其他章节和大纲）
4. **大纲重新生成保护**：`GenerateOutlineAction` 和 `generate_outline` 工具在存在已确认章节时拒绝执行（防止覆盖已完成内容），追加章节需使用「生成后续大纲」
5. **多轮消息保真**：Agent Loop 通过 `CallAPIMessages`/`CallAPIStreamMessages` 传递完整角色化消息历史，不再扁平化为单条 user 消息

### API 调用重试

所有 API 函数的第一个参数为 `context.Context`，支持任务取消。`CallAPIWithRetry` / `CallAPIStreamWithRetry` 为重试 + 指数退避（最大 30s），检查 `ctx.Err()` 实现取消，`time.Sleep` 替换为 `select { case <-time.After(d): case <-ctx.Done(): return }` 模式。带 `Log` 后缀的变体通过 SSE 推送重试信息。

致命错误检测：`isFatalAPIError` 检测 HTTP 401/403/404 等不可恢复错误，立即停止重试；网络超时/连接重置等瞬时错误会继续重试。`validateAPIConfig` 在任务开始前检查 BaseURL 和 Model 是否为空。

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
| GET | `/api/projects` | 同步 | 列出所有项目 |
| POST | `/api/projects` | 同步 | 创建新项目 |
| GET | `/api/projects/current` | 同步 | 获取当前项目名 |
| POST | `/api/projects/select` | 同步 | 切换到指定项目 |
| DELETE | `/api/projects/{name}` | 同步 | 删除项目 |
| GET | `/api/config/api` | 同步 | 获取 API 配置 |
| PUT | `/api/config/api` | 同步 | 保存 API 配置 |
| GET | `/api/config` | 同步 | 获取故事配置 |
| PUT | `/api/config` | 同步 | 保存故事配置 |
| GET | `/api/progress` | 同步 | 获取进度 |
| DELETE | `/api/progress` | 同步 | 重置进度 |
| GET | `/api/status` | 同步 | 获取状态摘要 |
| POST | `/api/outline/generate` | 异步 | 生成大纲（存在已确认章节时返回 409 拒绝） |
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
| POST | `/api/chapter/revise` | 异步 | 修订当前审核中章节 |
| POST | `/api/chapter/revise/{num}` | 异步 | 定向最小化修订指定章节（含已确认章节，不影响其他章节） |
| POST | `/api/chapter/polish` | 异步 | 去AI味（需启用 polish 类技能） |
| DELETE | `/api/chapter` | 同步 | 删除最后章节 |
| DELETE | `/api/chapters/from/{num}` | 同步 | 从第 N 章删除到末尾 |
| DELETE | `/api/outline` | 同步 | 删除大纲 |
| POST | `/api/task/stop` | 同步 | 停止当前运行的任务 |
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
| `ChapterRevision` | `chapter_revision` | 章节定向最小化修订 |
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
| `{{.Title}}` | `preferUserValue(cfg.Story.Title, state.Title)` | 小说标题（优先用户配置） |
| `{{.ChapterNum}}` | `ch.Num` | 章节编号 |
| `{{.CorePrompt}}` | `state.CorePrompt` | 核心写作提示词 |
| `{{.StorySynopsis}}` | `preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)` | 故事梗概（优先用户配置） |
| `{{.HistorySummary}}` | `buildHistorySummary()` | 最近 5 章摘要 |
| `{{.ChapterTitle}}` | `ch.Title` | 本章标题 |
| `{{.ChapterOutline}}` | `ch.Outline` | 本章大纲（修订时附加用户修改意见） |
| `{{.WritingStyle}}` | `cfg.Story.WritingStyle` | 写作风格（始终使用当前配置） |
| `{{.CharacterContext}}` | `buildCharacterContext()` | 结构化角色详情（从 settings 匹配） |
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

前端使用 Vite 5 + Svelte 4 + Tailwind CSS 4 + DaisyUI 5 构建，产物输出到 `frontend/dist/`，通过 `//go:embed frontend/dist` 内嵌到 Go 二进制。主题使用 xianii 暗色主题（定义在 `src/app.css` 的 `@plugin "daisyui/theme"` 块中）。

- **页面**：`config`（配置直接保存 + 角色管理 + 世界观管理）、`outline`（大纲直接操作 + 内联编辑 + 导入续写）、`writing`（写作直接操作 + 定向修订 + 导出 TXT）、`relations`（关系图谱 Canvas）、`skills`（技能管理）
- **状态管理**：Svelte stores（`src/lib/stores.js`），包含 progress、config、settings、taskRunning 等全局状态
- **路由**：hash 路由（`src/lib/router.js`），`currentPage` store + `window.hashchange` 监听
- **API 调用**：`api(method, url, body)` 封装 fetch（`src/lib/api.js`）
- **SSE**：`connectSSE()` 建立 EventSource 连接，12 种事件类型自动更新 stores（`src/lib/sse.js`）；任务成功完成以 toast 提示（不弹全屏遮罩）
- **开发模式**：`task dev:frontend` 启动 Vite dev server（端口 5173），代理 `/api` → `:48090`，支持 HMR 热重载
- **关系图谱**：`ForceGraph` 类，纯 Canvas 力导向布局，支持拖拽节点、悬浮 tooltip
- **聊天**：会话列表 + 停止按钮 + 任务状态/日志区 + 消息区 + 工具调用卡片（中文工具名、危险工具高亮、running/done 状态区分）+ 智能自动滚动 + 失败重试 banner
- **交互原则**：所有核心操作（生成/确认/修订/删除/保存）均为直接按钮 + API 调用，不依赖 AI 聊天间接执行；破坏性操作前端用 `ConfirmModal` 二次确认

## 重要约束

1. **零外部依赖**：仅使用 Go 标准库，不要引入第三方包
2. **前端构建**：前端在 `frontend/` 目录中使用 Svelte 组件开发，`npm run build` 产物输出到 `frontend/dist/`，不要拆分构建产物
3. **嵌入式文件**：前端通过 `//go:embed frontend/dist` 嵌入，skill 文件通过 `//go:embed embeds/skills` 嵌入，修改后需重新编译
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
