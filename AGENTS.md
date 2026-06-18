# AGENTS.md — AI 小说生成器项目指南

> **重要**：当对项目进行任何修改（代码、配置、前端、提示词等）后，必须同步更新本文件，确保文档与项目实际情况完全一致。

## 项目概述

单二进制 Go Web 应用，Go 后端零外部依赖（仅标准库），通过 OpenAI 兼容 API 自动生成长篇小说。前端使用 Vite + Svelte 4 + DaisyUI 4 构建，产物通过 `embed.FS` 内嵌到二进制中。

- **Go 版本**：1.25.1
- **模块名**：`showmethestory`
- **默认端口**：`:48090`（可通过 `PORT` 环境变量覆盖）
- **前端**：Vite 5 + Svelte 4 + Tailwind CSS 4 + DaisyUI 5（xianii 暗色主题）
- **项目目录**：`storys/`（程序同目录下，每个故事项目一个子目录）
- **多语言**：每个项目在创建时选择 `zh` / `en`，决定 AI 提示词、生成正文、内置技能与 Agent 系统提示；前端 UI 语言独立可切换
- **许可证**：MIT（见根目录 `LICENSE`）
- **文档**：根目录 [`README.md`](README.md)（中文）+ [`README.en.md`](README.en.md)（英文，首行链接互通）

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
                  ├─ postprocess.go ← 全书优化（诊断/核查/路线图/按章执行）+ postprocess.json 持久化
                  └─ filesys.go     ← 文件操作抽象
```

## 文件清单与职责

| 文件 | 职责 |
|------|------|
| `main.go` | 入口，确定程序目录（`progDir`），创建 `storys/` 目录，加载 API 配置，启动 Web 服务器（无项目选择状态） |
| `config.go` | `APIConfig`（含 `ContextBudgetTokens` 全书优化上下文预算）、`Config`（含 `SkillConfig` + `Language`）、`StoryConfig`、`PromptsConfig` 结构体，Load/Save 函数，`DefaultConfigForLang(lang)`、`NormalizeLanguage`、`applyDefaults(lang)` 按语言选择默认 prompts |
| `state.go` | `Progress`、`ChapterState`、`Foreshadow` 结构体，`LoadProgress`、`SaveProgress`（原子写入）、`ChapterMarkdownPath`、`SaveChapterMarkdown(projectDir, ...)`、`ForeshadowRoadmapPath`（项目目录 `Foreshadows.md`） |
| `api.go` | `CallAPI`/`CallAPIMessages`（**内部优先流式缓冲**，失败时回退 `callAPIMessagesSync`）、`CallAPIStream`/`CallAPIStreamMessages`（流式，含 `stream_options.include_usage`）、`CallAPIWithRetry`/`CallAPIWithRetryLog`（无限重试）、`CallAPIStreamWithRetry`/`CallAPIStreamWithRetryLog`，`validateAPIConfig`、`isFatalAPIError`（401/403/404 致命，网络超时可重试）；所有调用经 `taskCtx` 时自动累计 token（优先 API `usage`，否则 rune 估算） |
| `outline.go` | `generateOutline`、`reviseOutline`、`GenerateOutlineAction`（存在已确认章节时拒绝整体重新生成）、`ReviseOutlineAction`、`ConfirmOutlineAction`、`EditChapterOutline`、`cleanJSONResponse` |
| `writing.go` | `GenerateChapterAction`（含写前大纲一致性检查，共 5 步；第 5 步更新伏笔并落盘 `Foreshadows.md`）、`ReviseChapterAction`/`ReviseSpecificChapterAction`（修订后同步更新伏笔）、`ConfirmChapterAction`、`PolishChapterAction`、`SmoothTransitionsAction`（批量优化已确认章节衔接，逐章最小化重写开头、逐章落盘）、`parseFactCheckResult`（JSON 优先 + 字符串 fallback）、`checkOutlineConsistency`（写前检查本章大纲与已写剧情冲突，冲突时最小化修订本章大纲）、章节内容生成/摘要/事实核查/流式输出、`stripChapterMetaProse`（生成/修订/润色后剔除首尾元信息行）、`buildHistorySummary`、`buildPreviousChapterTail`（上一章尾部约 800 字注入写作 prompt）、`buildOutlineConstraints`（全书章节脉络反向约束：后续 10 章大纲防提前出现 + 前文大纲防一次性事件重复，注入写作与事实核查 prompt）、`appendIfMissingPlaceholder`（老项目持久化旧模板缺新占位符时把上下文块追加到渲染结果末尾兜底）、`splitChapterOpening` |
| `foreshadow.go` | `SuggestForeshadows`、`UpdateForeshadows`、伏笔格式化注入、伏笔告警、`BuildForeshadowRoadmapMarkdown`、`SaveForeshadowRoadmap`、`syncForeshadowsAfterChapter`、`NextForeshadowID` |
| `foreshadow_consistency.go` | `CheckForeshadowOutlineConsistency`、`RunForeshadowOutlineCheckAndSave`（大纲/伏笔变更后自动检查，报告写入 `progress.last_foreshadow_outline_report`） |
| `writing_conflict.go` | `analyzeWritingConflict`、`WritingConflictError`、事实核查多次失败后的根因分析与用户处理选项 |
| `continue.go` | `AnalyzeExistingContent`、`ImportContinueAction`、`GenerateContinuationOutline`、`splitContentByChapters` |
| `reconcile.go` | `ReconcileSettingsAction`、`regeneratePendingOutlines`、设定协调逻辑 |
| `settings.go` | `Character`、`WorldviewEntry`、`Organization`、`Relation`、`ProjectSettings` 结构体，`LoadProjectSettings`、`SaveProjectSettings`、`buildCharacterContext`、`buildWorldviewContext` |
| `skills.go` | `Skill`、`SkillConfig` 结构体，`LoadBuiltinSkills`、`LoadProjectSkills`、`MergeSkills`、`GetEnabledSkills`、`GetEnabledSkillsByCategory`、`FormatSkillsContent`，`//go:embed embeds/skills` |
| `agent.go` | `Tool`、`AgentContext`、`AgentStep`、`ToolCall` 结构体，`RunAgentLoop`（多轮消息历史 + 双语 tool 结果标签）、工具调用解析、内置工具集（读/写角色/世界观/章节等）、`buildAgentSystemPromptZH`/`buildAgentSystemPromptEN` 按项目语言选择系统提示、`requireConfirm`（破坏性工具需 `confirm: true`） |
| `chat.go` | `ChatSession`、`ChatMessage`（含 `tool_result_key`/`tool_result_args`）、`ChatSessionIndex` 结构体，Load/Save/Delete |
| `logger.go` | `LogBroadcaster`；`LogEntry` 含 `msg_key`/`msg_args`；`InfoKey`/`SuccessKey`/…；`ToolCallEnd` 含 `result_key`/`result_args`；其余 SSE 事件方法同前 |
| `postprocess.go` | `PostProcessState`/`RoadmapItem` 结构体，`LoadPostProcess`/`SavePostProcess`（`postprocess.json`）、`buildPostProcessBundle`（设定+摘要+全文组装与长文策略：全文/摘要模式）、`DiagnoseBookAction`、`ConsistencyCheckBookAction`（超长书按卷分段）、`BuildRoadmapAction`、`FullPostProcessAnalyzeAction`（诊断→核查→路线图）、`ExecuteRoadmapAction`（可选前置衔接优化 + 逐条定向修订/润色 + diff 节选） |
| `handlers.go` | `Handlers` 结构体（含项目管理字段 `progDir`/`projectName`/`projectMu`、自动确认开关 `autoConfirm`、`postprocess`/`postprocessPath`）、`projectDir()` 帮助函数、项目切换 `switchProject()`、`ensureProject()` 检查、`rejectIfTaskRunning()`（任务运行期间编辑类端点返回 409）、所有 HTTP handler（含 `PostChapterPolish` 单章去AI味、`PostChapterReviseSpecific` 定向修订、`PostChaptersSmoothTransitions` 批量衔接优化、全书优化 `GetPostProcess`/`PostPostProcessDiagnose`/`PostPostProcessConsistency`/`PostPostProcessRoadmap`/`PutPostProcessRoadmap`/`PostPostProcessExecute`/`DeletePostProcess`、`GetAutoConfirm`/`PutAutoConfirm`）、`PostChapterGenerate` 自动确认循环（开启时每章生成后自动确认并继续下一章）、`tryStartTask`/`endTask`/`startChildWork` 互斥、项目管理 handler（`GetProjects`/`PostProject`/`PostProjectSelect`/`GetProjectCurrent`/`DeleteProject`） |
| `web.go` | 路由注册（含项目管理端点、`/api/autoconfirm`）、CORS/日志中间件、静态文件服务、`startWebServer`、项目管理 handler（`GetProjects`/`PostProject`/`GetProjectCurrent`/`PostProjectSelect`/`DeleteProject`） |
| `tokens.go` | `TaskTokenUsage` 任务级 token 累计器（context 挂载）、`withTaskTokens`/`taskTokensFromContext`、throttled SSE 推送 |
| `prompts.go` | `RenderPrompt`（`{{.KeyName}}` 替换）、`DefaultPromptsZH` 变量（所有内置中文提示词模板）、`DefaultPromptsForLang(lang)` |
| `prompts_en.go` | `DefaultPromptsEN`：16 个 prompt 字段全量英文模板（与中文一一对应） |
| `locale.go` | `LangZH`/`LangEN` 常量、`localeFromRequest` 从 `X-UI-Locale`/`Accept-Language`/`?locale=` 解析、`errorCatalog` 双语错误表、`T(lang, key, args)`（同时查 `messageCatalog` + `errorCatalog`）、`systemPrompts` 内联 system prompt 集中表、`SystemPromptFor(lang, key)`、`Handlers.writeErrorReq` 本地化错误响应 |
| `messages.go` | `messageCatalog`：`log.*` SSE 日志 + `agent.*` 工具状态消息双语表（Go 侧 `%s`/`%d` 模板） |
| `agent_i18n.go` | Agent 工具 i18n 辅助：`agentMsg(ctx, key, args…)`（按项目语言生成 AI 可读文本并记录 key）、`agentErr`、`AgentContext` 临时 toolMsgKey/Args |
| `i18n_inject.go` | 注入块的双语版本：`buildOutlineConstraintsForLang`、`buildPreviousChapterTailForLang`、`buildHistorySummaryForLang`、`buildCharacterContextForLang`、`buildWorldviewContextForLang`、`formatActiveForeshadowsForChapterLang`、`formatChapterLine`、`formatForeshadowsForPromptLang` |
| `filesys.go` | `writeFileImpl`、`deleteFileImpl`、`renameFileImpl` |
| `skills.go` | `Skill`（含 `Lang` 字段）、`SkillConfig` 结构体，`LoadBuiltinSkills`、`LoadProjectSkills`、`MergeSkills`、`GetEnabledSkills`、`GetEnabledSkillsByCategory`、`FilterSkillsByLang(skills, projectLang)`、`FormatSkillsContent`（按 skill 语言选择双语 header）、`//go:embed embeds/skills` |
| `embeds/skills/*.md` | 内置 Skill 文件（YAML frontmatter `lang: zh|en` + prompt body），通过 `//go:embed` 嵌入；中文：`humanizer-zh.md` / `story-deslop.md` / `writing-craft.md`；英文：`humanizer-en.md` / `story-deslop-en.md` / `writing-craft-en.md` |
| `.github/workflows/release.yml` | GitHub Actions 发布流程：推送 `v*` tag 时校验 tag 在 main 分支上，构建前端 + 交叉编译 5 个目标（linux/windows/macOS × amd64/arm64，windows 仅 amd64），打包 tar.gz/zip 并用 `gh` 创建 Release |

### 前端文件（`frontend/`）

| 文件 | 职责 |
|------|------|
| `package.json` | 前端依赖：Svelte 4、Vite 5、Tailwind CSS 4、DaisyUI 5、marked + dompurify（聊天 markdown 渲染） |
| `vite.config.js` | Vite 配置：`@tailwindcss/vite` 插件、Svelte 插件、dev server 代理 `/api` → `:48090`、构建输出到 `dist/` |
| `svelte.config.js` | Svelte 预处理器配置 |
| `index.html` | 入口 HTML，`data-theme="xianii"` |
| `src/main.js` | Svelte 应用挂载点 |
| `src/app.css` | 全局样式：Tailwind 指令 + 自定义滚动条/toast 动画 |
| `src/App.svelte` | 根组件：Header（项目badge + 项目语言 badge ZH/EN + 「切换 / 新建项目」按钮（任务运行时禁用）+ 阶段badge + 章节进度badge + AI思考中badge + 右侧 UI 语言切换按钮中 / EN） + 顶部导航（配置/大纲/写作/伏笔/图谱/技能，带图标） + 页面路由 + LogPanel + Toast 容器；初始加载若有当前项目则 `setLocale(project.language)` |
| `src/lib/api.js` | `api(method, url, body)` — fetch 封装，自动带 `X-UI-Locale`/`Accept-Language` 头，错误消息走 `translateServerMessage` |
| `src/lib/router.js` | `currentPage` store + hash 路由监听 |
| `src/lib/stores.js` | 全局 Svelte stores（progress、config、settings、postprocess、taskRunning、taskTokenUsage、autoConfirm、lastFailedTask、`projectLanguage` 等）+ toast/log 管理 |
| `src/lib/sse.js` | `connectSSE()` — EventSource `?locale=`；`log` → `formatLogEntry`；`tool_call_end` → `formatToolResult`；任务名 `task.<name>`；流式节流/尾部窗口等同前 |
| `src/lib/i18n/index.js` | `uiLocale`、`t`/`translate`（`{name}`）、`formatKeyedMessage`/`formatLogEntry`/`formatToolResult`（服务端 key + `{0}`）、`translateServerMessage` legacy 兜底 |
| `src/lib/i18n/zh.js`, `en.js` | 扁平 key 字典；新增可见文案必须同时在两个文件加 key |
| `src/pages/Projects.svelte` | 项目选择页：新建项目（名称全宽 + 中文/EN 分段按钮选语言，POST 时携带 `language`）+ 项目列表（每项显示语言 badge，可选择/删除）；选中项目后 `setLocale(project.language)` |
| `src/pages/Config.svelte` | 配置页：API 配置（含上下文预算 tokens）、故事配置（直接 PUT 保存 + 关键设定变更时提示协调）、写作风格与叙述视角、角色管理、世界观管理、组织管理（卡片 + 成员勾选）、关系管理（卡片 + 源/目标实体选择）；任务运行时所有输入控件禁用 |
| `src/pages/Outline.svelte` | 大纲页：直接操作按钮（生成/确认/修订意见/删除/生成后续大纲）+ 导入续写 + pending 章节内联编辑 + 流式预览 |
| `src/pages/Writing.svelte` | 写作页：章节列表（状态点）+ 直接操作（生成/确认/修改意见/去AI味，自动区分当前章修订与定向修订）+ 事实核查冲突处理面板（`pending_writing_conflict`，可选修改大纲/伏笔/重试/保留稿进入审核）+ 自动确认模式开关（toggle，随时可开关）+ 伏笔追踪摘要卡片（活跃/超期/临近回收）+ 优化章节衔接（进度卡片工具栏小按钮，已确认 ≥ 2 章时显示）+ 导出 TXT + 复制 + 上下章导航 + 流式尾部窗口展示（含「仅显示最新内容」提示；任务进行中当前章显示 taskTokenUsage，空闲时显示正文字数）+ rAF 自动滚动（自动确认模式下自动跟随正在生成的章节）+ 全书完成后展示 `PostProcessPanel` |
| `src/components/TaskTokenBadge.svelte` | 任务 token 展示组件（`↑ prompt ↓ completion tokens`），供 ChatPanel / App 顶栏 / Writing 页复用 |
| `src/pages/Foreshadows.svelte` | 伏笔页：统计概览 + AI 设计伏笔 + 手动 CRUD + AI 建议确认面板（SSE `foreshadow_suggestions`）+ 伏笔-大纲冲突报告卡片（`last_foreshadow_outline_report`）+ 列表/章节时间线/路线图文档三视图 + 复制/下载 `Foreshadows.md` |
| `src/components/PostProcessPanel.svelte` | 全书优化面板：开始全书分析（诊断+核查+路线图）/ 重新核查 / 重新生成路线图 / 清空；诊断与核查报告 Markdown 展示；优化工单表格（勾选、编辑意见、执行选项、diff 对比弹窗） |
| `src/pages/Relations.svelte` | 图谱页：Canvas 力导向图谱（ForceGraph 类），支持拖拽、滚轮缩放（以光标为中心，0.3x–3x）、hover 高亮（强调 hover 节点与其连线，次强调直接相邻节点，其余淡化） |
| `src/pages/Assistant.svelte` | 助理页：聊天会话列表 + 消息区 + 工具调用卡片 + 流式回复 |
| `src/pages/Skills.svelte` | 技能页：技能表格 + toggle 开关 |
| `src/components/ChatPanel.svelte` | 右侧聊天面板；任务日志走 `formatLogEntry`；工具结果走 `formatToolResult`；其余同前 |
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
- **后端**：所有编辑类同步端点（配置/角色/世界观/组织/关系/伏笔/技能/大纲编辑/会话删除等）在 handler 开头调用 `rejectIfTaskRunning(w)`，任务运行期间返回 409，防止意外提交编辑造成数据竞争
- **前端**：所有按钮使用 `disabled={$taskRunning}` 禁用，所有输入控件（input/textarea/select）同样 `disabled={$taskRunning}`
- **前端**：发送消息前检查 `$taskRunning`，API 返回 409 时显示错误提示

### 自动确认模式

`Handlers.autoConfirm`（`taskMu` 保护）为运行时开关，不持久化。`GET/PUT /api/autoconfirm` 读取/切换，任务运行期间也可随时开关。开启后 `PostChapterGenerate` 的任务 goroutine 进入循环：生成章节 → 若开关仍开启则 `ConfirmChapterAction` 自动确认 → 继续生成下一章，直到全部完成、开关被关闭（当前章生成完后停在 review 状态）、任务被取消或出错。整个循环在同一个任务锁内执行，期间仍受任务互斥保护。`GET /api/status` 返回 `auto_confirm` 及任务运行中的 `token_usage` 字段。前端开关位于写作页进度卡片（toggle），开启时流式输出自动跟随正在生成的章节。

### 流式输出节流 + 尾部窗口（前端性能）

后端逐 token 推送 `content_chunk`/`chat_chunk`。节流只能降低更新频率，若 store 中保存完整流式全文，每次刷新仍需对全文重新渲染/排版（成本随长度线性增长，总成本 O(n²)），长章节会把主线程占满直至页面无响应。因此采用多层防护：

- **节流缓冲**：`sse.js` 将 chunk 先累积到本地缓冲区，每 150ms 批量刷入 store
- **尾部窗口**：章节流式全文只存 `sse.js` 模块级变量，`streamingContent` store 仅保留尾部约 3000 字符，每次刷新渲染成本恒定；写作页流式期间显示「仅显示最新内容」提示，生成结束后由 progress 重新拉取展示全文
- **rAF 滚动**：写作页自动滚动合并到 `requestAnimationFrame`，每帧最多一次
- **任务 token 追踪**：`tryStartTask()` 将 `TaskTokenUsage` 挂到 `taskCtx`；`api.go` 每次 LLM 调用累计 prompt/completion（优先 API `usage`，否则 rune×1.5 估算）；throttled SSE `token_usage`（约 2s）+ 前端每 2s poll `GET /api/status` 兜底；ChatPanel 任务栏、App 顶栏「AI思考中」、Writing 当前章均展示 `↑ ↓ tokens`
- **progress 去抖**：`progress_update` 事件触发的 `/api/progress` 拉取（含全书正文的大 JSON）500ms 内合并为一次，`task_end` 时立即刷新

`stream_start` 事件（每次章节流式输出开始时由后端发出）会清空缓冲与已生成字数计数，避免事实核查重试或自动连写时新旧内容叠加。

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
- 去AI味（`POST /api/chapter/polish`）：加载所有 enabled 的 `polish` 类 skill；全书优化执行时可选附加去 AI 味
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

`CallAPIStream` 返回流式响应，通过 `onChunk` 回调实时推送每个 token。`ContentChunk` SSE 事件用于前端实时渲染；token 累计经 `TaskTokenUsage` 推送 `token_usage` SSE（约 2s 节流）。

### 大纲反向约束 + 写前一致性检查

防止「后续章节安排的人物/事件提前出现」与「一次性事件（初遇、身份揭示）重复发生」的三层防线（两者是同一根因：写前面章节时看不到后续大纲，事件意外提前发生，后面又按大纲再写一遍）：

1. **事前（写前检查）**：`GenerateChapterAction` 第 1 步调用 `checkOutlineConsistency`（第 1 章跳过），AI 对照前情提要 + 上一章结尾检查本章大纲是否已与实际剧情冲突（如大纲安排初遇但前文已认识），冲突时用 `revised_outline` 最小化替换本章大纲并立即落盘，再开始写正文；检查失败不阻塞（按原大纲继续）
2. **事中（写作约束）**：`buildOutlineConstraints` 生成「全书章节脉络」块注入写作 prompt——后续 10 章大纲（严禁提前发生/剧透）+ 前文全部章节大纲（一次性事件不得重复发生）
3. **事后（核查闭环）**：事实核查 prompt 注入本章大纲 + 章节脉络，核查范围含「提前引入后续章节事件」「一次性事件重复发生」两项，FAIL 触发最多 3 次自动重写

兜底防线：`ChapterSummary` 模板含【人物动态】条目（出场人物、初次见面、身份揭示等一次性事件），若某事件已意外提前发生，后续章节的前情提要会明确记录，配合「严格承接前情」要求处理为延续而非重新发生。

老项目兼容：prompts 随 `config.json` 持久化，旧模板缺新占位符时 `appendIfMissingPlaceholder` 把约束块（事实核查还含补充核查规则）追加到渲染结果末尾，保证老项目同样生效。

### 章节状态机

```
pending → writing → review → accepted
                           ↗
                    （修改后回到 review）
```

### 伏笔生命周期

```
（AI 建议 → 用户确认 / 手动创建 / 助理 create_foreshadow）
  → planted → progressing → resolved
                         ↘ abandoned
```

写作时：`formatActiveForeshadowsForChapter` 注入活跃伏笔到 `ChapterWriting` prompt。  
章末：`GenerateChapterAction` / `ReviseChapterAction` / `ReviseSpecificChapterAction` 调用 `syncForeshadowsAfterChapter`（AI 更新状态 + events + resolution），并写入项目目录 `Foreshadows.md` 路线图。  
超期：`BuildForeshadowWarnings` 在日志面板告警（超过预计回收章 3 章以上）。

前端「伏笔」页提供列表、按章节时间线、Markdown 路线图预览；SSE `foreshadow_suggestions` 触发建议确认面板。

### 进度持久化

每个关键步骤后立即保存 `progress.json`。API 配置保存 `api.json`，故事配置保存 `config.json`。设定保存 `settings.json`。使用原子写入（先写 `.tmp` 再 rename）。

### 结构化设定

`settings.json` 存储 `ProjectSettings`，包含 `Characters`、`Worldview`、`Organizations`、`Relations` 四个数组。这些设定在章节写作时通过 `buildCharacterContext`/`buildWorldviewContext` 注入到 prompt 中。设定也支持 AI 自动生成（`POST /api/settings/ai-generate`）。

### 会话管理

聊天会话存储为项目目录 `sessions/` 下的 JSON 文件。`sessions/index.json` 为会话索引。每个会话文件名为 `{id}.json`。使用 `writeFileAtomic` 保持一致性。

## 全书优化流程

```
写作页全书已确认 → 「全书优化」面板
 → POST /api/postprocess/diagnose（异步，同一任务锁）
    1. DiagnoseBookAction：设定+摘要+全文（超预算时仅摘要模式）→ 诊断报告
    2. ConsistencyCheckBookAction：按卷（15万字/卷）核查 → 核查报告
    3. BuildRoadmapAction：报告 → 结构化工单 JSON → postprocess.json
 → 用户审阅报告、勾选/编辑工单
 → POST /api/postprocess/execute（异步）
    可选前置 SmoothTransitionsAction
    逐条 ExecuteRoadmapAction：同章多条工单合并为一次 ReviseSpecificChapterAction / PolishChapterAction
    每条完成后保存 diff 节选（前 500 字）+ 更新工单状态
 → 可随时 POST /api/task/stop 取消（已完成项不丢失）
```

- 上下文预算：`api.json` 的 `context_budget_tokens`（默认 900000），配置页可编辑
- 数据持久化：项目目录 `postprocess.json`（报告、工单、执行状态）
- 单独重跑：`POST /api/postprocess/consistency`（仅核查）、`POST /api/postprocess/roadmap`（仅路线图）

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
| GET | `/api/chapter/conflict` | 同步 | 获取待处理写作冲突（`pending_writing_conflict`） |
| POST | `/api/chapter/conflict-resolve` | 同步 | 处理写作冲突（`retry`/`force_review`/`dismiss`） |
| POST | `/api/foreshadows/outline-check` | 异步 | 手动触发伏笔-大纲一致性检查 |
| POST | `/api/chapter/confirm` | 同步 | 确认章节 |
| POST | `/api/chapter/revise` | 异步 | 修订当前审核中章节 |
| POST | `/api/chapter/revise/{num}` | 异步 | 定向最小化修订指定章节（含已确认章节，不影响其他章节） |
| POST | `/api/chapter/polish` | 异步 | 单章去AI味（`{"num":N}` 可选，需启用 polish 类技能；已确认章节润色后保持 accepted 状态） |
| POST | `/api/chapters/smooth-transitions` | 异步 | 批量优化已确认章节衔接（逐章检查上一章结尾与本章开头，仅生硬时最小化重写开头片段，逐章落盘可随时停止） |
| GET | `/api/postprocess` | 同步 | 获取全书优化状态（报告 + 工单 + 元信息） |
| DELETE | `/api/postprocess` | 同步 | 清空全书优化报告与工单 |
| PUT | `/api/postprocess/roadmap` | 同步 | 更新优化工单（勾选/编辑意见/执行选项） |
| POST | `/api/postprocess/diagnose` | 异步 | 全书优化分析（诊断 → 一致性核查 → 生成路线图，需全书已确认） |
| POST | `/api/postprocess/consistency` | 异步 | 仅重新运行全书一致性核查 |
| POST | `/api/postprocess/roadmap` | 异步 | 根据已有报告重新生成路线图 |
| POST | `/api/postprocess/execute` | 异步 | 执行已勾选工单（可选前置衔接优化 + 逐章修订/润色，逐条落盘可随时停止） |
| DELETE | `/api/chapter` | 同步 | 删除最后章节 |
| DELETE | `/api/chapters/from/{num}` | 同步 | 从第 N 章删除到末尾 |
| DELETE | `/api/outline` | 同步 | 删除大纲 |
| POST | `/api/task/stop` | 同步 | 停止当前运行的任务 |
| GET | `/api/autoconfirm` | 同步 | 获取自动确认模式开关状态 |
| PUT | `/api/autoconfirm` | 同步 | 切换自动确认模式（任务运行期间也可随时开关） |
| GET | `/api/foreshadows` | 同步 | 获取伏笔列表 |
| GET | `/api/foreshadows/roadmap` | 同步 | 获取伏笔路线图 Markdown（含项目内 `Foreshadows.md` 路径） |
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
| `stream_start` | `{chapter_idx}` | 一次新的章节流式输出开始（前端清空流式缓冲，避免事实核查重试/自动连写时内容叠加） |
| `content_chunk` | `{chapter_idx, text}` | 流式生成 token |
| `token_usage` | `{prompt_tokens, completion_tokens}` | 任务级 token 累计（约 2s 节流；含流式与非流式步骤） |
| `foreshadow_suggestions` | `ForeshadowSuggestion[]` | 伏笔建议结果 |
| `foreshadow_outline_conflicts` | `ForeshadowOutlineReport` | 伏笔与大纲一致性检查发现冲突 |
| `writing_conflict` | `WritingConflict` | 事实核查多次失败且无法自动调和，等待用户选择处理方向 |
| `continue_analysis` | `ContinueAnalysis` | 续写分析结果 |
| `settings_reconciled` | `{explanation, changed_fields}` | 设定协调完成 |
| `chat_chunk` | `{session_id, text}` | 助理流式回复 |
| `tool_call_start` | `{session_id, tool_name, args}` | Agent 工具调用开始 |
| `tool_call_end` | `{session_id, tool_name, result}` | Agent 工具调用结束 |
| `polish_result` | `{chapter_idx, text}` | 去AI味结果 |
| `postprocess_report` | `{type, content}` | 全书诊断/核查报告（type: diagnosis/consistency） |
| `postprocess_roadmap` | `PostProcessState` | 优化路线图生成完成 |
| `postprocess_item_done` | `RoadmapItem` | 单条工单执行完成（含 diff 节选） |
| `postprocess_update` | `{book_complete, state}` | 全书优化状态更新 |

## PromptsConfig 字段

| 字段 | JSON key | 用途 |
|------|----------|------|
| `OutlineGeneration` | `outline_generation` | 大纲生成 |
| `ChapterWriting` | `chapter_writing` | 章节创作（含章节脉络反向约束） |
| `ChapterRevision` | `chapter_revision` | 章节定向最小化修订 |
| `ChapterSummary` | `chapter_summary` | 摘要提炼（含【人物动态】一次性事件记录） |
| `FactCheck` | `fact_check` | 事实核查（含提前引入/重复发生检测） |
| `OutlineRevision` | `outline_revision` | 大纲修订 |
| `ForeshadowPlanning` | `foreshadow_planning` | 伏笔规划 |
| `ForeshadowUpdate` | `foreshadow_update` | 伏笔状态更新 |
| `ContentAnalysis` | `content_analysis` | 续写内容分析 |
| `ContinuationOutlineGeneration` | `continuation_outline_generation` | 续写大纲生成 |
| `SettingsReconciliation` | `settings_reconciliation` | 设定协调 |
| `TransitionSmoothing` | `transition_smoothing` | 章节衔接优化（判断 + 最小化重写开头片段，无需修改时输出 NO_CHANGE） |
| `OutlineConsistencyCheck` | `outline_consistency_check` | 写前大纲一致性检查（对照前情提要 + 上一章结尾，冲突时输出最小化修订后的本章大纲） |
| `ForeshadowOutlineConsistency` | `foreshadow_outline_consistency` | 伏笔与完整大纲一致性检查 |
| `WritingConflictAnalysis` | `writing_conflict_analysis` | 事实核查多次失败后的根因分析与处理建议 |
| `BookDiagnosis` | `book_diagnosis` | 全书完稿诊断报告（只诊断不改写） |
| `BookConsistencyCheck` | `book_consistency_check` | 全书一致性核查（超长书按卷分段） |
| `BookRoadmap` | `book_roadmap` | 诊断+核查报告 → 结构化工单 JSON |

新增 prompt 模板时需要：(1) 在 `PromptsConfig` 添加字段，(2) 在 `DefaultPrompts` 添加默认值，(3) 在 `applyDefaults` 添加 fallback。

## ChapterWriting 模板占位符

| 占位符 | 来源 | 说明 |
|--------|------|------|
| `{{.Title}}` | `preferUserValue(cfg.Story.Title, state.Title)` | 小说标题（优先用户配置） |
| `{{.ChapterNum}}` | `ch.Num` | 章节编号 |
| `{{.CorePrompt}}` | `state.CorePrompt` | 核心写作提示词 |
| `{{.StorySynopsis}}` | `preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)` | 故事梗概（优先用户配置） |
| `{{.HistorySummary}}` | `buildHistorySummary()` | 最近 5 章摘要 |
| `{{.PreviousEnding}}` | `buildPreviousChapterTail()` | 上一章结尾原文约 800 字（段落对齐，含说明包装；第 1 章或上一章无内容时为空） |
| `{{.ChapterTitle}}` | `ch.Title` | 本章标题 |
| `{{.ChapterOutline}}` | `ch.Outline` | 本章大纲（修订时附加用户修改意见） |
| `{{.WritingStyle}}` | `cfg.Story.WritingStyle` | 写作风格（始终使用当前配置） |
| `{{.WritingPOV}}` | `cfg.Story.WritingPOV` | 叙述视角（如第一人称女主、第三人称限知；老模板缺占位符时由 `appendIfMissingPlaceholder` 追加） |
| `{{.CharacterContext}}` | `buildCharacterContext()` | 结构化角色详情（从 settings 匹配） |
| `{{.WorldviewContext}}` | `buildWorldviewContext()` | 结构化世界观详情（从 settings 匹配） |
| `{{.TargetWords}}` | snapshot | 每章目标字数 |
| `{{.Foreshadows}}` | `formatActiveForeshadowsForChapter()` | 活跃伏笔上下文 |
| `{{.OutlineConstraints}}` | `buildOutlineConstraints()` | 全书章节脉络反向约束（后续 10 章大纲防提前出现 + 前文大纲防一次性事件重复；无内容时为空；老模板缺占位符时追加到 prompt 末尾） |

## 内置 Skill 文件

| 文件 | ID | 语言 | 分类 | 说明 |
|------|----|------|------|------|
| `embeds/skills/humanizer-zh.md` | `humanizer-zh` | zh | polish | 23 条中文 AI 痕迹禁用模式 + 高频短语替换表（top 50） + 口语化/格式规范规则 |
| `embeds/skills/story-deslop.md` | `story-deslop` | zh | polish | 6-Gate 中文检测流程 + AI 味检测报告模板 + 真人写作基准表 |
| `embeds/skills/writing-craft.md` | `writing-craft` | zh | writing | 章首钩子 7 式 + 章尾钩子 13 式 + 爽点密度 + 节奏控制（中文网文范式） |
| `embeds/skills/humanizer-en.md` | `humanizer-en` | en | polish | **本地化非翻译**：针对英文 LLM 输出的 30 条禁用模式（delve/tapestry/em-dash 滥用/said-bookisms/filtering 等）+ 高频替换表 + 风格规则（缩写/Anglo-Saxon 词/具体名词）+ 英文格式规范（US/UK 引号、em-dash 间距、ellipsis） |
| `embeds/skills/story-deslop-en.md` | `story-deslop-en` | en | polish | 6-Gate 英文检测：slop-word 密度（per 1000 words）/英文陈词/em-dash & 三联结构/语音差异/emotion-by-body/节奏与开篇结尾，按英文 trade fiction 基线 |
| `embeds/skills/writing-craft-en.md` | `writing-craft-en` | en | writing | 章首钩子 7 式 + 章尾钩子 13 式 + Sanderson 的 promise/progress/payoff + Swain scene-and-sequel 节奏 + 对话经济 + 角色 voice + 按场景类型节奏表（英文 trade fiction 范式，非"爽点"框架） |

Skill 文件格式：YAML frontmatter（`---` 分隔，含 `lang: zh|en`，无 `lang` 视为语言无关）+ Markdown body。`LoadAllSkills` 通过 `FilterSkillsByLang` 按 `cfg.Language` 过滤可见 skill。前端通过 `GET /api/skills` 获取列表，`PUT /api/skills/{id}/toggle` 切换启用状态。

**英文 skill 是本地化设计而非中文翻译**：英文 LLM 的 AI 痕迹（delve / tapestry / em-dash 泛滥 / said-bookisms / "in a world where" 等）与中文 LLM 的痕迹（宛如 / 不禁 / 微微 / 缓缓 / 心中暗道 等）不同；写作技法框架也按英文 trade fiction 约定（Sanderson、Swain）而非中文网文的爽点密度。

## 前端架构

前端使用 Vite 5 + Svelte 4 + Tailwind CSS 4 + DaisyUI 5 构建，产物输出到 `frontend/dist/`，通过 `//go:embed frontend/dist` 内嵌到 Go 二进制。主题使用 xianii 暗色主题（定义在 `src/app.css` 的 `@plugin "daisyui/theme"` 块中）。

- **页面**：`config`（配置直接保存 + 角色管理 + 世界观管理 + 组织管理（卡片 + 角色成员勾选）+ 关系管理（卡片 + 源/目标实体下拉，实体覆盖角色/组织/世界观，值编码为 `type:id`））、`outline`（大纲直接操作 + 内联编辑 + 导入续写）、`writing`（写作直接操作 + 定向修订 + 自动确认模式开关 + 伏笔追踪摘要 + 导出 TXT）、`foreshadows`（伏笔 CRUD + AI 建议确认 + 列表/时间线/路线图三视图）、`relations`（关系图谱 Canvas）、`skills`（技能管理）
- **状态管理**：Svelte stores（`src/lib/stores.js`），包含 progress、config、settings、taskRunning、taskTokenUsage（任务 token 累计）、autoConfirm（自动确认模式）、foreshadowSuggestions/foreshadowShowSuggestions（AI 伏笔建议待确认）等全局状态
- **路由**：hash 路由（`src/lib/router.js`），`currentPage` store + `window.hashchange` 监听
- **API 调用**：`api(method, url, body)` 封装 fetch（`src/lib/api.js`）
- **SSE**：`connectSSE()` 建立 EventSource 连接，14 种事件类型自动更新 stores（`src/lib/sse.js`）；content_chunk/chat_chunk 经 150ms 节流缓冲批量刷入；`token_usage` 更新 taskTokenUsage，任务运行中每 2s poll `/api/status` 兜底；任务成功完成以 toast 提示（不弹全屏遮罩）
- **Markdown 渲染**：助理消息通过 `src/lib/markdown.js`（marked + DOMPurify）渲染为 HTML，样式在 `app.css` 的 `.md-body` 块中定义
- **开发模式**：`task dev:frontend` 启动 Vite dev server（端口 5173），代理 `/api` → `:48090`，支持 HMR 热重载
- **关系图谱**：`ForceGraph` 类，纯 Canvas 力导向布局，支持拖拽节点、滚轮缩放（以光标为中心）、悬浮 tooltip 与 hover 高亮（hover 节点及连线强调、相邻节点次强调、无关元素淡化）
- **聊天**：会话列表 + 停止按钮 + 任务状态/日志区 + 消息区 + 工具调用卡片（中文工具名、危险工具高亮、running/done 状态区分）+ 智能自动滚动 + 失败重试 banner
- **交互原则**：所有核心操作（生成/确认/修订/删除/保存）均为直接按钮 + API 调用，不依赖 AI 聊天间接执行；破坏性操作前端用 `ConfirmModal` 二次确认
- **i18n 模块**：`src/lib/i18n/index.js` 提供 `uiLocale` store（writable，写入 `localStorage`）、`setLocale(lang)`、`getLocale()`、`t` 派生 store（`$t('key', params)`）、`translate(key, params, lang)` 命令式版本、`translateServerMessage(msg, lang)` 把后端中文消息映射到英文；字典在 `src/lib/i18n/zh.js` 与 `en.js`（扁平 key 表，缺 key 时回退中文），插值占位符为 `{name}`

## 多语言架构（i18n）

### 项目语言 vs UI 语言

| 维度 | 存储 | 作用范围 | 可变更 |
|------|------|---------|--------|
| **项目语言** `cfg.Language` | `config.json` 的 `language` 字段（`"zh"` / `"en"`） | AI 提示词模板、注入块（角色/世界观/章节脉络/上一章结尾/历史摘要/伏笔）、所有 system prompt、Agent 系统提示与工具反馈、技能筛选、生成正文 | **否**（创建时选定） |
| **UI 语言** `uiLocale` | 浏览器 `localStorage`（`showmethestory.uiLocale`） | 前端文案、API 错误信息、SSE 日志 | **是**（Header 切换；切换 / 选择项目时会同步重置为该项目语言） |

### 后端关键文件

- [`config.go`](config.go)：`Config.Language` 字段、`DefaultConfigForLang(lang)`、`NormalizeLanguage(lang)`、`PromptsConfig.applyDefaults(lang)` 按语言回填空字段
- [`prompts.go`](prompts.go)：`DefaultPromptsZH`（原默认值改名）
- [`prompts_en.go`](prompts_en.go)：`DefaultPromptsEN` 全量英文模板（16 个字段，与中文一一对应）
- [`messages.go`](messages.go)：`messageCatalog`（`log.*` / `agent.*`）；新增后端日志或 Agent 状态消息时在此加 key，并同步 `frontend/src/lib/i18n/zh.js` + `en.js`（位置占位 `{0}`/`{1}`）
- [`locale.go`](locale.go)：`errorCatalog` + `T()`；所有 API 错误走 `writeErrorReq(w, r, code, key, args…)`
- [`logger.go`](logger.go)：`InfoKey`/`SuccessKey`/… 替代硬编码中文；SSE `LogEntry.msg_key` + `msg_args`
- [`agent_i18n.go`](agent_i18n.go) + [`agent.go`](agent.go)：工具状态返回用 `agentMsg(ctx, "agent.xxx", …)`；读工具的数据型返回仍按项目语言格式化，不带 key
- [`chat.go`](chat.go)：`ChatMessage.tool_result_key`/`tool_result_args` 持久化；`AgentStep` 同字段
- [`frontend/src/lib/i18n/index.js`](frontend/src/lib/i18n/index.js)：`formatLogEntry` / `formatToolResult` 按 `uiLocale` 渲染服务端 key
- [`frontend/src/components/ChatPanel.svelte`](frontend/src/components/ChatPanel.svelte)：任务日志与工具结果走 key 化渲染

### 前端关键文件

- [`frontend/src/lib/i18n/zh.js`](frontend/src/lib/i18n/zh.js) / [`frontend/src/lib/i18n/en.js`](frontend/src/lib/i18n/en.js)：UI 文案用 `{name}`；镜像 `log.*`/`agent.*` 服务端 key 用 `{0}`/`{1}`
- [`frontend/src/lib/api.js`](frontend/src/lib/api.js)：所有请求带 `X-UI-Locale`；`writeErrorReq` 已按请求语言返回错误，``translateServerMessage`` 仅 legacy 兜底
- [`frontend/src/lib/sse.js`](frontend/src/lib/sse.js)：`formatLogEntry` / `formatToolResult`
- [`frontend/src/lib/stores.js`](frontend/src/lib/stores.js)：新增 `projectLanguage` writable
- [`frontend/src/App.svelte`](frontend/src/App.svelte)：Header 显示项目语言 badge（ZH/EN）+ UI 语言切换按钮（中 / EN）；选择/创建项目后自动 `setLocale(project.language)`
- [`frontend/src/pages/Projects.svelte`](frontend/src/pages/Projects.svelte)：新建项目表单名称全宽 + 中文/EN 分段按钮选语言，POST 时携带 `language`；列表项显示语言 badge

### 老项目兼容

- `config.json` 无 `language` 字段 → `NormalizeLanguage` 视为 `"zh"`
- 已有非空 `prompts` 字段 → `applyDefaults(lang)` 仅填空字段，**不**用 EN 模板覆盖
- 已有章节 / 设定不受影响，继续以原语言生成

## 重要约束

1. **零外部依赖**：Go 后端仅使用标准库，不要引入第三方包（前端 npm 依赖不受此限制）
2. **前端构建**：前端在 `frontend/` 目录中使用 Svelte 组件开发，`npm run build` 产物输出到 `frontend/dist/`，不要拆分构建产物
3. **嵌入式文件**：前端通过 `//go:embed frontend/dist` 嵌入，skill 文件通过 `//go:embed embeds/skills` 嵌入，修改后需重新编译
4. **配置文件 gitignore**：`*.json` 被 gitignore，不要提交配置/进度/设定/会话文件
5. **提示词用 `{{.KeyName}}`**：不是 Go `text/template`，是简单字符串替换
6. **异步任务互斥**：同一时间只能有一个 AI 任务运行（`tryStartTask`/`endTask`）
7. **原子写入**：配置和进度文件使用 `writeFileAtomic`（先写 `.tmp` 再 rename）
8. **双语界面**：UI 文案走 `$t('key', {name})`；后端日志/Agent 状态走 `messageCatalog` key + `InfoKey`/`agentMsg`；API 错误走 `writeErrorReq` + `errorCatalog`；新增 key 须同步 `messages.go`（或 `errorCatalog`）与 `zh.js`/`en.js`
9. **Skill 可选性**：所有 skill 默认禁用，功能性 AI 不注入任何 skill，除非作者显式启用
10. **多语言一致**：新增 prompt 模板必须同时在 `prompts.go`（`DefaultPromptsZH`）和 `prompts_en.go`（`DefaultPromptsEN`）补齐；新增注入块文本必须在 `i18n_inject.go` 处理两种语言；新增内联 system prompt 必须挂到 `locale.go` 的 `systemPrompts` map

## 修改检查清单

完成代码修改后，必须执行以下检查：

1. `go build -o show-me-the-story.exe .` 编译通过
2. 确认无未使用的 import 或变量
3. 如果修改了 API 端点，确认 `web.go` 中路由已注册
4. 如果新增了 prompt 模板，确认 `config.go` 和 `applyDefaults` 已更新
5. 如果修改了 SSE 事件，确认前端已添加对应监听
6. 如果新增了 Skill 文件，确认在 `embeds/skills/` 目录中，并设置 `lang: zh|en` frontmatter（语言无关的写 `lang: ""` 或省略）
7. 如果新增了 prompt / system prompt / 注入块，确认中英双语都已补齐（见多语言一致约束）
8. 如果新增了前端可见文案，确认 `zh.js` 与 `en.js` 同步加 key
9. **同步更新本 AGENTS.md 文件** + 必要时同步更新 [`README.md`](README.md) 与 [`README.en.md`](README.en.md)
