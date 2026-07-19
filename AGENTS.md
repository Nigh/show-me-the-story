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

编译前务必确认 `go build ./...` 无报错，并运行 `go test ./...`（各 internal 包共 70+ 单元测试）。

## 架构概览（Go 包结构）

Go 代码按依赖层次拆分为 `internal/` 下的单向依赖包（上层可依赖下层，反向禁止）：

```
main.go                      入口：progDir 解析、api.json 加载、//go:embed frontend/dist、组装 httpapi
└── internal/
    ├── httpapi/             HTTP 层：web.go 路由注册 + handlers.go 全部 handler、任务互斥、SSE 端点
    ├── agent/               Agent Loop 引擎 + 内置工具集（全局助理用）
    ├── story/               领域层：进度/章节存储、大纲、写作、伏笔、卷、导入、设定、技能、会话、全书优化
    ├── llm/                 OpenAI 兼容 API 客户端：重试、流式、致命错误检测、token 统计、JSON 提取
    ├── config/              APIConfig / Config / StoryConfig / PromptsConfig + 中英默认提示词模板
    ├── sse/                 LogBroadcaster：SSE 事件广播（领域无关，事件负载为 any）
    ├── i18n/                语言常量、errorCatalog/messageCatalog 双语表、T()、systemPrompts
    ├── prose/               正文字数统计 CountProseUnits（与前端 proseUnits.js 同口径）
    └── fsutil/              文件写入原语（WriteFileAtomic 等）
```

依赖方向：`httpapi → agent → story → {llm, config, sse, i18n, prose, fsutil}`；另有 `llm → {config, sse}`、`sse → i18n`、`config → {i18n, fsutil}`。`story` 包内部仍按功能域分文件（outline / writing / foreshadow / arcs / importer / …）。

请求处理流：

```
用户浏览器 ←→ internal/httpapi（web.go 路由 + handlers.go）
   ├─ 同步端点：直接返回 JSON
   ├─ 异步端点：tryStartTask() → go func() { defer h.endTask(); story.XxxAction(...) } → SSE 推送
   └─ SSE (internal/sse)：实时日志/进度/事件推送到前端
```

## 文件清单与职责

| 文件 | 职责 |
|------|------|
| `main.go` | 入口，确定程序目录（`progDir`），创建 `storys/` 目录，加载 API 配置（`llm.EnsureContextBudget` 补齐上下文预算），`//go:embed frontend/dist` 嵌入前端产物并传给 `httpapi.StartWebServer`；`var version = "dev"` 通过 CI `-ldflags` 注入实际版本号 |
| `internal/fsutil/fsutil.go` | 文件写入原语：`WriteFile`/`Delete`/`Rename`/`WriteFileAtomic`（先写 `.tmp` 再 rename） |
| `internal/prose/units.go` | `CountProseUnits`（CJK +1；连续字母数字 token +1，内部 `.` `,` `-` `#` 连接；全角字母数字视同半角；标点/空白断词不计数；中英文共用） |
| `internal/i18n/locale.go` | `LangZH`/`LangEN` 常量、`NormalizeLanguage`、`FromRequest` 从 `X-UI-Locale`/`Accept-Language`/`?locale=` 解析、`errorCatalog` 双语错误表、`T(lang, key, args)`（同时查 `messageCatalog` + `errorCatalog`）、`MsgArgs`、`systemPrompts` 内联 system prompt 集中表、`SystemPromptFor(lang, key)` |
| `internal/i18n/messages.go` | `messageCatalog`：`log.*` SSE 日志 + `agent.*` 工具状态消息双语表（Go 侧 `%s`/`%d` 模板） |
| `internal/config/config.go` | `APIConfig`（含 `URLStrict` 严格 URL 模式、`ContextBudgetTokens` 全书优化上下文预算、`DefaultContextBudgetTokens` 常量）、`Config`（含 `ProjectFormatVersion`、`SkillConfig` + `Language`）、`StoryConfig`、`PromptsConfig`、`SkillConfig` 结构体，Load/Save 函数（`LoadAPIConfig`/`LoadConfig`/`SaveConfig`），`DefaultConfigForLang(lang)`、`ApplyDefaults(lang)` 按语言选择默认 prompts |
| `internal/config/prompts.go` | `RenderPrompt`（`{{.KeyName}}` 替换）、`DefaultPromptsZH` 变量（所有内置中文提示词模板）、`DefaultPromptsForLang(lang)` |
| `internal/config/prompts_en.go` | `DefaultPromptsEN`：全量英文模板（与中文一一对应） |
| `internal/sse/logger.go` | `LogBroadcaster`；`LogEntry` 含 `msg_key`/`msg_args`；`InfoKey`/`SuccessKey`/…；`ToolCallEnd` 含 `result_key`/`result_args`；`Format`（SSE wire 格式）；领域事件方法（`ForeshadowSuggestions`/`ConfigChangeProposal`/`PostProcess*` 等）负载类型为 `any`，保持包领域无关 |
| `internal/llm/api.go` | `resolveChatCompletionsURL`/`normalizeURL`（`url_strict` 时仅补 `/chat/completions`；否则路径含 `/vN` 只补 `/chat/completions`，裸域名补 `/v1/chat/completions`）、`Message`、`CompletionResult`（含 `FinishReason`）、`CallAPI`/`CallAPIMessages`（**内部优先流式缓冲**，失败时回退 `CallAPIMessagesSync`）、`CallAPIStream`/`CallAPIStreamMessages`（流式，解析 `finish_reason` + `stream_options.include_usage`）、`CallAPIWithRetry`/`CallAPIWithRetryLog`（无限重试 + `RetryWaitTime` 指数退避）、`ValidateConfig`、`IsFatalAPIError`（401/403/404 致命，网络超时可重试）、`FetchModelContextWindow`/`EnsureContextBudget`；所有调用经 `taskCtx` 时自动累计 token（优先 API `usage`，否则 rune 估算） |
| `internal/llm/jsonextract.go` | `ExtractJSON`/`WalkJSONStructure`：从自由格式模型输出中定位首个完整 JSON 对象（字符串感知的花括号匹配），story 事实核查与 agent 工具解析共用 |
| `internal/llm/tokens.go` | `TaskTokenUsage` 任务级 token 累计器（context 挂载）、`WithTaskTokens`/`TaskTokensFromContext`、`EstimateTokensFromRunes`（rune×1.5 估算）、throttled SSE 推送 |
| `internal/llm/api_url_test.go` | `resolveChatCompletionsURL` 表驱动测试（z.ai v4、strict、DeepSeek、完整 URL） |
| `internal/story/state.go` | `Progress`（含 `Arcs []Arc` 卷结构）、`Arc`（卷：ID/Title/Goal/StartCh/EndCh/Summary）、`ChapterState`（含 `WordCount` 字数、`ContentRev` 内容修订号）、`Foreshadow`、`MemoryEntry`（含 API 响应专用 `Snippet`）结构体，`LoadProgress`、`SaveProgress`（v3 分文件存储，见 `storage.go`）、`ChapterMarkdownPath`、`SaveChapterMarkdown(projectDir, ...)`、`ForeshadowRoadmapPath`（项目目录 `Foreshadows.md`） |
| `internal/story/storage.go` | **v3 存储层**：章节正文按章存 `chapters/NNNNNN.json`，`progress.json` 只存元数据（不含正文）；`saveChapterFiles`（fnv 内容哈希 `HashContent` 脏检查，仅重写变更章节 + 清理孤儿文件）、`loadChapterContents`、`ResetProgressFiles`（重置进度含章节目录）、`ProgressView`（API 响应视图：剥离正文、附 `word_count`/`content_rev`、解析记忆 `snippet`） |
| `internal/story/blocks.go` | **v3 Block 模型**：`Block{ID,Type,Text}`（type: paragraph/dialogue/scene_break 仅展示提示）；`SyncChapterBlocks`（从 Content 派生 blocks，未变段落 ID 稳定，`\n\n` 分段、无空行退化 `\n`，sep 存 `BlockSep`）、`rebuildContentFromBlocks`、`UpdateBlock`/`DeleteBlock`/`InsertBlockAfter`（块编辑后重建 Content）、`ReviseBlockAction`（复用 `ChapterSegmentRevision` prompt 对单 block AI 修订）。内存中 Content 仍是唯一事实源，AI 流程不感知 blocks |
| `internal/story/arcs.go` | **v3 层级大纲（卷）**：`Arc` 辅助（`arcForChapterNum`/`arcChapters`/`arcCompleted`/`ArcIndexByID`）、`GenerateArcSkeletonAction`（一次小调用生成全书卷骨架，`assignArcRanges` 把各卷章数换算为连续区间并强制总和等于 `chapter_count`；存在已确认/写作中章节时拒绝）、`GenerateArcOutlineAction`（按卷分批生成章纲，注入 `buildPreviousArcContext` 卷摘要前情 + `buildFutureArcsBlock` 后续卷约束）、`AppendArcAction`（追加新卷 + 生成章纲，失败回滚；无限连载增量入口）、`EnsureArcSummaries`（懒生成已完结卷的卷摘要） |
| `internal/story/outline.go` | `generateOutline`（注入 settings 角色列表 + 按 `target_words_per_chapter` 计算大纲字数下限，不足时自动重试）、`reviseOutline`、`GenerateOutlineAction`（存在已确认章节时拒绝整体重新生成；完成后 `runOutlinePostProcessChecks`）、`ReviseOutlineAction`、`ConfirmOutlineAction`、`EditChapterOutline`（`pending`/`writing`/`review` 可编辑，`accepted` 拒绝）、`cleanJSONResponse`、`GenerateContinuationOutline`（生成后续大纲） |
| `internal/story/outline_helpers.go` | `calcOutlineLengthRange`、`formatCharacterListForOutline`、`validateOutlineChapterLengths`、`buildOutlineDerivedCharacterContext`（写作时注入未登记大纲人物 stub） |
| `internal/story/outline_character.go` | `CheckOutlineCharacterConsistency`、`RunOutlineCharacterCheckAndSave`、`runOutlinePostProcessChecks`（伏笔-大纲 + 大纲人物双检查） |
| `internal/story/writing.go` | `GenerateChapterAction`（开头懒调用 `EnsureArcSummaries`；含写前大纲一致性检查，共 6 步；第 2 步经 `generateChapterContentWithLengthControl` 控字数；第 5 步更新伏笔并落盘 `Foreshadows.md`；第 6 步维护叙事记忆）、`ReviseChapterAction`/`ReviseSpecificChapterAction`（修订后同步更新伏笔与记忆；修改意见含 `> ` 引用行时经 `extractQuotedSentences`/`findParagraphsContaining`/`reviseChapterSegment` 只改匹配自然段，失败回退整章修订）、`ConfirmChapterAction`、`PolishChapterAction`、`SmoothTransitionsAction`（批量优化已确认章节衔接）、`parseFactCheckResult`（JSON 优先 + 字符串 fallback）、`checkOutlineConsistency`（写前检查本章大纲与已写剧情冲突）、`stripChapterMetaProse`、`appendIfMissingPlaceholder`（老项目旧模板缺新占位符时兜底追加）、`splitChapterOpening`、`syncMemoryAfterChapter`、`calcMemoryMaxTokens` |
| `internal/story/writing_length.go` | `calcChapterLengthRange`（±1000 字或 ±15% 取较大者）、`generateChapterContentWithLengthControl`（生成/重写间 `maybeUpdateBestDraft` 保留最佳稿；略超/略低 soft 容忍跳过 adjust；仍超限 `log.chapter_length_off_range` 警告，不阻塞自动确认）、老模板 `finalizeChapterWritingPrompt` 兜底 |
| `internal/story/writing_delete.go` | `ResolveDeleteChapterTarget` / `DeleteFrontierChapter`：清除**写作前沿**章节正文，同步回退指针、清除该章叙事记忆；`FormatWritingFrontierInfo` 注入 Agent 系统提示 |
| `internal/story/writing_conflict.go` | `analyzeWritingConflict`、`WritingConflictError`、事实核查多次失败后的根因分析与用户处理选项；`ResolveForceReviewIndex` / `PromoteWritingToReview`（冲突章或孤儿 `writing` 前沿章 → `review`） |
| `internal/story/foreshadow.go` | `SuggestForeshadows`、`UpdateForeshadows`、伏笔格式化注入、伏笔告警、`BuildForeshadowRoadmapMarkdown`、`SaveForeshadowRoadmap`、`syncForeshadowsAfterChapter`、`NextForeshadowID`、`ForeshadowStatusLabel`；以及 `CheckForeshadowOutlineConsistency`、`RunForeshadowOutlineCheckAndSave`（大纲/伏笔变更后自动检查，报告写入 `progress.last_foreshadow_outline_report`） |
| `internal/story/importer.go` | **v3 导入流水线**：`SplitImportContent`（本地切章：标题正则 + 无标题按 ~6000 字块切分）、`BuildImportPreview`、`ImportState`/`Load/SaveImportState`（`import.json` 断点）、`ImportStartAction`（切章落盘为 accepted → 元信息分析 → 逐章 outline/summary，每章一个检查点）、`ImportResumeAction`（断点续跑）、`createImportArcs`（≥40 章自动分卷 30 章/卷） |
| `internal/story/reconcile.go` | `ReconcileSettingsAction`（保持用户提交的 `newSettings`，AI 调整差异写入 pending 提案）、`regeneratePendingOutlines`、设定协调逻辑 |
| `internal/story/config_guard.go` | `ConfigFieldChange`/`PendingConfigChanges` 结构体，`CollectStoryConfigConflicts`、`applyStoryConfigMerge`、`applyOutlineMetaWithGuard`、`Load/SavePendingConfigChanges`（`pending_config_changes.json`）、`ApplySelectedPendingChanges`、`SyncProgressMetaFromStory` |
| `internal/story/settings.go` | `Character`、`WorldviewEntry`、`Organization`、`Relation`、`ProjectSettings` 结构体（含 `NextCharacterID` 等 ID 分配），`LoadProjectSettings`、`SaveProjectSettings` |
| `internal/story/skills.go` | `Skill`（含 `Lang` 字段）结构体（`SkillConfig` 在 `internal/config`），`LoadBuiltinSkills`、`LoadProjectSkills`、`MergeSkills`、`GetEnabledSkills`、`GetEnabledSkillsByCategory`、`FilterSkillsByLang(skills, projectLang)`、`FormatSkillsContent`（按 skill 语言选择双语 header）、`//go:embed embeds/skills` |
| `internal/story/editing.go` | `EditChapterContent` 章节正文局部编辑（`replace_lines`/`replace_text`/`insert_after_line`/`append`），`EditChapterContentRequest` 结构体，`EditOp` 常量，`FindChapterIdx` 辅助函数 |
| `internal/story/chat.go` | `ChatSession`、`ChatMessage`（含 `tool_result_key`/`tool_result_args`）、`ToolCall`、`ChatSessionIndex` 结构体，Load/Save/Delete、`ChatSessionsDir`/`GenerateSessionID`/`GenerateChatTitle` |
| `internal/story/postprocess.go` | `PostProcessState`/`RoadmapItem` 结构体，`LoadPostProcess`/`SavePostProcess`（`postprocess.json`）、`buildPostProcessBundle`（设定+摘要+全文组装与长文策略）、`DiagnoseBookAction`、`ConsistencyCheckBookAction`（超长书按卷分段）、`BuildRoadmapAction`、`FullPostProcessAnalyzeAction`（诊断→核查→路线图）、`ExecuteRoadmapAction`（可选前置衔接优化 + 逐条定向修订/润色 + diff 节选）、`IsBookFullyAccepted` |
| `internal/story/inject.go` | 注入块的双语版本：`buildOutlineConstraintsForLang`（有卷摘要的已完结卷压缩为一行卷摘要）、`buildPreviousChapterTailForLang`、`buildHistorySummaryForLang`、`buildCharacterContextForLang`、`buildWorldviewContextForLang`、`formatActiveForeshadowsForChapterLang`、`formatChapterLine`、`formatForeshadowsForPromptLang`、`buildMemoryForLang`（叙事记忆注入）、`extractSnippet`（按段落位置截取原文片段）、`formatMemoryForUpdatePrompt` |
| `internal/story/*_test.go` | 领域层单测：存储 roundtrip/脏检查/孤儿清理、Block ID 稳定性与 CRUD、卷区间换算与上下文压缩、导入切章/断点、引用式段落修订、字数区间、删章目标解析等 |
| `internal/agent/agent.go` | `Tool`、`AgentContext`、`AgentStep` 结构体（`ToolCall` 别名指向 `story.ToolCall`），`RunAgentLoop`（多轮消息历史 + 双语 tool 结果标签）、工具调用解析（`llm.ExtractJSON` 字符串感知；`finish_reason==length` 且 tool_call 未完整时 `agent.output_truncated` 报错，不修复截断 JSON）、内置工具集（读/写角色/世界观/章节等）、`buildAgentSystemPromptZH`/`buildAgentSystemPromptEN`、`update_project_config` 覆盖已填字段需 `confirm_overwrite: true`、`requireConfirm`（破坏性工具需 `confirm: true`）；文件内含原 `agent_i18n.go` 的 `agentMsg`/`agentErr` i18n 辅助 |
| `internal/agent/agent_truncated_test.go` | Agent 工具调用解析单元测试：截断不修复、`ExtractJSON` 字符串感知、`finish_reason` 截断检测 |
| `internal/httpapi/handlers.go` | `Handlers` 结构体（含项目管理字段 `progDir`/`projectName`/`projectMu`、自动确认开关 `autoConfirm`、`postprocess`/`postprocessPath`）、`projectDir()` 帮助函数、项目切换 `switchProject()`、`ensureProject()` 检查、`rejectIfTaskRunning()`（任务运行期间编辑类端点返回 409）、`writeErrorReq` 本地化错误响应、所有 HTTP handler（块编辑/卷/导入/全书优化/自动确认等）、`PostChapterGenerate` 自动确认循环、`tryStartTask`/`endTask`/`startChildWork` 互斥、项目管理 handler、`GetVersion` |
| `internal/httpapi/project_compat.go` | 项目格式只读检测：新工程以 `config.json.project_format_version=3` 为契约；旧版内嵌章节正文或未识别布局标记为不兼容。选择前拒绝，保证不会创建目录或写回配置；无标记但完整 v3 分章布局可作为历史 v3 项目兼容打开并补写标记 |
| `internal/httpapi/web.go` | 路由注册（含项目管理端点、`/api/autoconfirm`、`/api/version`）、CORS/日志中间件、静态文件服务（`StartWebServer` 接收 main 传入的 `fs.FS`） |
| `internal/story/embeds/skills/*.md` | 内置 Skill 文件（YAML frontmatter `lang: zh|en` + prompt body），通过 `//go:embed` 嵌入；中文：`humanizer-zh.md` / `story-deslop.md` / `writing-craft.md`；英文：`humanizer-en.md` / `story-deslop-en.md` / `writing-craft-en.md` |
| `.github/workflows/release.yml` | GitHub Actions 发布流程：推送 `v*` tag 时校验 tag 在 main 分支上，构建前端 + 交叉编译 5 个目标（linux/windows/macOS × amd64/arm64，windows 仅 amd64），打包 tar.gz/zip 并用 `gh` 创建 Release；通过 `-ldflags "-X main.version=${GITHUB_REF_NAME}"` 注入版本号 |

### 前端文件（`frontend/`）

| 文件 | 职责 |
|------|------|
| `package.json` | 前端依赖：Svelte 4、Vite 5、Tailwind CSS 4、DaisyUI 5、marked + dompurify（聊天 markdown 渲染） |
| `vite.config.js` | Vite 配置：`@tailwindcss/vite` 插件、Svelte 插件、dev server 代理 `/api` → `:48090`、构建输出到 `dist/` |
| `svelte.config.js` | Svelte 预处理器配置 |
| `index.html` | 入口 HTML，`data-theme="xianii"` |
| `src/main.js` | Svelte 应用挂载点 |
| `src/app.css` | 全局样式：Tailwind 指令 + 自定义滚动条/toast 动画 |
| `src/App.svelte` | 根组件：Header（项目badge + 项目语言 badge ZH/EN + 版本号badge + 新版本更新提示（非dev版本检查GitHub releases）+ 「切换 / 新建项目」按钮（任务运行时禁用）+ 阶段badge + 章节进度badge + AI思考中badge + 右侧 UI 语言切换按钮中 / EN） + 左侧竖排导航（配置/大纲/写作/伏笔/记忆/图谱/技能，图标+文字，约 176px）+ 中间页面内容 + 右侧 ChatPanel + Toast 容器；初始加载若有当前项目则 `setLocale(project.language)` |
| `src/lib/apiUrl.js` | `resolveChatCompletionsURL`：与后端 `api.go` 同逻辑的 URL 预览（配置页展示实际请求地址） |
| `src/lib/api.js` | `api(method, url, body)` — fetch 封装，自动带 `X-UI-Locale`/`Accept-Language` 头，错误消息走 `translateServerMessage` |
| `src/lib/router.js` | `currentPage` store + hash 路由监听 |
| `src/lib/stores.js` | 全局 Svelte stores（progress、config、settings、postprocess、taskRunning、taskTokenUsage、autoConfirm、lastFailedTask、`projectLanguage`、`pendingConfigChanges`/`showConfigChangePanel`、`apiTestResult` LLM 连接测试结果持久化 等）+ toast/log 管理 |
| `src/lib/proseUnits.js` | `countProseUnits`：与后端 `prose_units.go` 同口径，供写作页章节/全书字数展示 |
| `src/lib/tokenPoll.js` | `TOKEN_POLL_INTERVAL_MS`：token poll 间隔与 TaskTokenBadge 数字线性动画时长共用 |
| `src/lib/sse.js` | `connectSSE()` — EventSource `?locale=`；`log` → `formatLogEntry`；`tool_call_end` → `formatToolResult`；任务名 `task.<name>`；流式节流/尾部窗口等同前 |
| `src/lib/i18n/index.js` | `uiLocale`、`t`/`translate`（`{name}`）、`formatKeyedMessage`/`formatLogEntry`/`formatToolResult`（服务端 key + `{0}`）、`translateServerMessage` legacy 兜底 |
| `src/lib/i18n/zh.js`, `en.js` | 扁平 key 字典；新增可见文案必须同时在两个文件加 key |
| `src/pages/Projects.svelte` | 项目选择页：新建项目（名称全宽 + 中文/EN 分段按钮选语言，POST 时携带 `language`）+ 项目列表（每项显示语言 badge，可选择/删除）；选中项目后 `setLocale(project.language)` |
| `src/pages/Config.svelte` | 配置页：API 配置（含 `url_strict` 严格 URL 模式、解析后 endpoint 预览、上下文预算 tokens、连接测试结果持久化展示——结果存 `apiTestResult` store 切页不丢失，成功/失败以文字+着色卡片与按钮描边展示，修改任一影响连接的字段后自动清除）、故事配置（直接 PUT 保存 + 关键设定变更时提示协调）、写作风格与叙述视角、AI 配置变更确认面板（`ConfigChangePanel`）、角色管理、世界观管理、组织管理（卡片 + 成员勾选）、关系管理（卡片 + 源/目标实体选择）；任务运行时所有输入控件禁用 |
| `src/pages/Outline.svelte` | 大纲页：直接操作按钮（生成/确认/修订意见/删除/生成后续大纲）+ **卷结构面板**（生成卷骨架、按卷生成/重生成章纲（可附本卷补充要求）、追加新卷）+ **导入流水线**（本地切章预览 → 开始导入 → 断点恢复横幅，`GET /api/import/status` 探测）+ `pending`/`writing`/`review` 章节内联编辑（写作冲突跳转时经 `sessionStorage` 聚焦冲突章）+ 流式预览 + 标题/梗概展示优先 config（`preferUserValue` 一致）+ `ConfigChangePanel` + 未登记大纲人物确认面板（SSE `outline_character_suggestions`） |
| `src/components/ConfigChangePanel.svelte` | AI 配置变更确认面板：展示 pending 提案（当前 vs 建议）、勾选采纳 / 全部忽略；SSE `config_change_proposal` 触发 |
| `src/pages/Writing.svelte` | 写作页（v3：正文按需经 `GET /api/chapters/{num}` 拉取，`content_rev` 变化时刷新缓存；字数展示用索引里的 `word_count`；导出走 `GET /api/export/txt`；正文以 block 列表渲染，hover 出现 编辑/AI 修订/插入/删除 工具条，内联编辑与段落级 AI 修订）：章节列表（状态点）+ 直接操作（生成/确认/修改意见/去AI味，自动区分当前章修订与定向修订）+ 正文框选后浮动「引用到修改意见」按钮（插入 `> ` 引用行，触发段落级修订）+ 事实核查冲突处理面板（`pending_writing_conflict`：改大纲/伏笔/重试/`force_review`；`dismiss`≡保留稿进入审核）+ 孤儿 `writing` 恢复条（无 conflict 记录时仍可重新生成或进入审核）+ 自动确认模式开关（toggle，随时可开关）+ 伏笔追踪摘要卡片（活跃/超期/临近回收）+ 优化章节衔接（进度卡片工具栏小按钮，已确认 ≥ 2 章时显示）+ 导出 TXT + 复制 + 上下章导航 + 流式尾部窗口展示（含「仅显示最新内容」提示；任务进行中当前章显示 taskTokenUsage，空闲时以 `countProseUnits` 显示正文字数）+ rAF 自动滚动（自动确认模式下自动跟随正在生成的章节）+ 全书完成后展示 `PostProcessPanel` |
| `src/components/TaskTokenBadge.svelte` | 任务 token 展示（`↑ prompt ↓ completion tokens`）；对 `taskTokenUsage` 更新做线性 rAF 插值，动画时长 = `TOKEN_POLL_INTERVAL_MS`；目标值低于当前显示值时该维度从 0 重新向上插值（新一段统计或估算修正）；供 ChatPanel / App 顶栏 / Writing 页复用 |
| `src/pages/Foreshadows.svelte` | 伏笔页：统计概览 + AI 设计伏笔 + 手动 CRUD + AI 建议确认面板（SSE `foreshadow_suggestions`）+ 伏笔-大纲冲突报告卡片（`last_foreshadow_outline_report`）+ 列表/章节时间线/路线图文档三视图 + 复制/下载 `Foreshadows.md` |
| `src/pages/Memory.svelte` | 叙事记忆页（只读）：从 `progress.memory_entries` 展示统计（条数/覆盖章节/token 上限/内容字数）+ 列表/按章节时间线两视图 + 分类/章节筛选 + 原文片段预览（v3：片段由后端在 `snippet` 字段解析下发）+ 刷新/复制 |
| `src/components/PostProcessPanel.svelte` | 全书优化面板：开始全书分析（诊断+核查+路线图）/ 重新核查 / 重新生成路线图 / 清空；诊断与核查报告 Markdown 展示；优化工单表格（勾选、编辑意见、执行选项、diff 对比弹窗） |
| `src/pages/Relations.svelte` | 图谱页：Canvas 力导向图谱（ForceGraph 类），支持拖拽、滚轮缩放（以光标为中心，0.3x–3x）、hover 高亮（强调 hover 节点与其连线，次强调直接相邻节点，其余淡化） |
| `src/pages/Assistant.svelte` | 助理页：聊天会话列表 + 消息区 + 工具调用卡片 + 流式回复 |
| `src/pages/Skills.svelte` | 技能页：技能表格 + toggle 开关 |
| `src/components/ChatPanel.svelte` | 右侧聊天面板；任务日志走 `formatLogEntry`；工具结果走 `formatToolResult`；其余同前 |
| `src/components/ConfirmModal.svelte` | 全局确认弹窗组件（替代浏览器 confirm） |
| `src/components/LogPanel.svelte` | 底部可折叠实时日志面板 |

## 关键设计模式

### 项目目录化

`main.go` 接受命令行参数 `os.Args[1]` 作为程序基础目录（`progDir`），默认为当前目录。在 `progDir` 下自动创建 `storys/` 目录，每个故事项目是 `storys/{projectName}/` 子目录。`api.json` 始终在 `progDir` 下（全局共享）。所有项目文件（`progress.json`、`config.json`、`settings.json`、`sessions/`）都在各自项目目录中。新建 v3 项目会在 `config.json` 写入 `project_format_version: 3`；项目列表只读检查此标记，旧版内嵌正文格式和未识别格式显示为不兼容且无法选择，避免 v3 重写旧工程。

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
        h.writeErrorReq(w, r, http.StatusConflict, "task_running")
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
- **任务 token 追踪**：`tryStartTask()` 将 `TaskTokenUsage` 挂到 `taskCtx`；`api.go` 每次 LLM 调用累计 prompt/completion（优先 API `usage`，否则 rune×1.5 估算）；throttled SSE `token_usage` + 前端 poll `GET /api/status` 兜底（间隔见 `frontend/src/lib/tokenPoll.js` 的 `TOKEN_POLL_INTERVAL_MS`）；`TaskTokenBadge` 对该间隔内数字做线性 rAF 插值，目标低于当前显示时该维度从 0 重启动画；ChatPanel 任务栏、App 顶栏「AI思考中」、Writing 当前章均展示 `↑ ↓ tokens`
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

### 用户已填配置保护（无字段锁）

`config.json` 的 `StoryConfig` 文本字段（type/title/writing_style/writing_pov/story_synopsis）非空即视为用户已填。AI 输出与已填值冲突时：

1. **默认保留用户值**，不静默覆盖 config 或 progress meta
2. **冲突写入** `pending_config_changes.json`，SSE 推送 `config_change_proposal`，配置页/大纲页 `ConfigChangePanel` 待用户勾选采纳
3. **空字段**仍允许 AI 直接填充（如首次生成梗概）
4. **助理** `update_project_config` 覆盖已填字段需 `confirm_overwrite: true`（须先在对话中征得用户同意）
5. **设定协调**保持用户刚保存的 `newSettings`，AI 兼容调整建议走 pending 提案

写作阶段 `preferUserValue(cfg.Story.*, state.*)` 与大纲页展示规则一致。

### Agent Loop

独立 `internal/agent` 包，`RunAgentLoop(goCtx context.Context, ctx *AgentContext, userMessage string, history []AgentStep, maxSteps int)` 函数实现工具调用循环，接受 `context.Context` 支持任务取消。最大工具调用步骤为 30（安全上限，AI 自然终止不受此限）。内置工具集包括：`read_characters`、`read_character`、`read_worldview`、`read_organizations`、`read_chapter`、`read_outline`、`read_foreshadows`、`search_project`、`create_character`、`update_character`、`create_worldview`、`update_worldview`、`delete_character`、`delete_worldview`、`create_organization`、`update_organization`、`delete_organization`、`create_relation`、`update_relation`、`delete_relation`、`read_project_config`、`update_project_config`、`generate_outline`、`confirm_outline`、`revise_outline`、`delete_outline`、`edit_chapter_outline`、`generate_chapter`、`confirm_chapter`、`edit_chapter_content`、`revise_chapter`、`delete_chapter`、`delete_chapters_from`、`suggest_foreshadows`、`create_foreshadow`、`update_foreshadow`、`delete_foreshadow`、`read_skills`、`toggle_skill`、`reset_progress`。仅全局助理使用 Agent Loop。

工具调用解析支持：`<tool_call>` XML 标签（含 JSON 或 XML 内容）、JSON 代码块、裸 JSON 对象（含 `name`/`tool` 键）。解析具有多级 fallback：`<tool_call>` 内 JSON → `<tool_call>` 内 XML 格式 → `</tool_call>` 之后的 JSON → 全文 JSON → `function.name()` 格式。`parseToolCallJSON` 遍历内容中所有 JSON 对象而非仅第一个。

### Agent 安全护栏

防止 AI 误删用户数据的多层防护：

1. **系统提示词安全规则**：`buildAgentSystemPrompt` 包含最高优先级的「安全规则」（修改 ≠ 删除）和「工具选择指南」，明确指示修改章节细节必须用 `revise_chapter` 而非删除重写；删除写作前沿单章用 `delete_chapter`（**禁止**误用 `delete_chapters_from`）；删更早章节及之后正文才用 `delete_chapters_from` 并须复述范围；**缩章/整本重生大纲**（尚无已确认章节）须 `update_project_config` + `generate_outline`，禁止 `revise_outline` 缩章、禁止 `delete_chapters_from` 减章、无需先 `delete_outline`
2. **破坏性工具二次确认**：`delete_chapter`、`delete_chapters_from`、`delete_outline`、`reset_progress` 必须传入 `confirm: true` 参数，否则返回警告信息要求 AI 先向用户确认
3. **`revise_chapter` 支持任意章节**：可选 `num` 参数，当前审核中章节走 `ReviseChapterAction`（完整流程），其他章节（含已确认）走 `ReviseSpecificChapterAction`（最小化定向修订，不影响其他章节和大纲）
4. **大纲重新生成保护**：`GenerateOutlineAction` 和 `generate_outline` 工具在存在已确认章节时拒绝执行（防止覆盖已完成内容），追加章节需使用「生成后续大纲」；`generate_outline` 会完全替换 pending 大纲，读取 `config.json` 的 `chapter_count` / `target_words_per_chapter`
5. **多轮消息保真**：Agent Loop 通过 `CallAPIMessages`/`CallAPIStreamMessages` 传递完整角色化消息历史，不再扁平化为单条 user 消息

面向用户的使用说明见 [`README.md`](README.md) / [`README.en.md`](README.en.md) 的「AI 助理」一节。

### API 调用重试

所有 API 函数的第一个参数为 `context.Context`，支持任务取消。`CallAPIWithRetry` 为重试 + 指数退避（`RetryWaitTime`，最大 30s），检查 `ctx.Err()` 实现取消，`time.Sleep` 替换为 `select { case <-time.After(d): case <-ctx.Done(): return }` 模式。带 `Log` 后缀的变体通过 SSE 推送重试信息。

致命错误检测：`llm.IsFatalAPIError` 检测 HTTP 401/403/404 等不可恢复错误，立即停止重试；网络超时/连接重置等瞬时错误会继续重试。`llm.ValidateConfig` 在任务开始前检查 BaseURL 和 Model 是否为空。

### 流式输出

`CallAPIStream` 返回流式响应，通过 `onChunk` 回调实时推送每个 token。`ContentChunk` SSE 事件用于前端实时渲染；token 累计经 `TaskTokenUsage` 推送 `token_usage` SSE（约 2s 节流）。

### 大纲生成约束 + 人物一致性

- **字数**：`calcOutlineLengthRange(target_words_per_chapter)` 计算每章大纲建议区间（默认约 target/20–target/8 字，最低 80–150）；`OutlineGeneration` / `ContinuationOutlineGeneration` / `OutlineRevision` 模板含 `{{.OutlineMinWords}}` / `{{.OutlineMaxWords}}` / `{{.CharacterList}}`；生成后 `validateOutlineChapterLengths` 校验，不足则自动重试一次（`outlineGenMaxAttempts=2`）
- **结构**：prompt 要求每章含场景、冲突、转折、出场人物、章末钩子
- **角色白名单**：生成时注入 `settings.json` 已登记角色；优先使用，新增须标注「首次登场」
- **生成后检查**：`runOutlinePostProcessChecks` = 伏笔-大纲检查 + `OutlineCharacterCheck`（AI + 「首次登场」启发式）；未登记人物 SSE `outline_character_suggestions`，大纲页可 `POST /api/outline/characters/confirm` 一键创建角色
- **写作兜底**：`buildCharacterContextForLang` 追加 `buildOutlineDerivedCharacterContext`，为未登记但大纲标注「首次登场」的人物注入临时设定块
- **老项目兼容**：持久化旧 prompt 缺新占位符时，`finalizeOutlinePrompt` 在末尾追加字数/结构/角色块（同 `appendIfMissingPlaceholder` 思路）

### 层级大纲（卷 / Arc，v3 超长篇支持）

1000+ 章的书无法一次生成完整大纲（输出 token 上限），也无法把全部前文大纲注入写作 prompt（输入 token 线性膨胀）。解决方案是两级结构：

```
卷骨架（一次小调用）        POST /api/arcs/skeleton
  Arc{title, goal, start_ch, end_ch}[]，各卷章数之和 == chapter_count
    ↓ 按卷分批（用户逐卷触发）  POST /api/arcs/{id}/outline
  本卷章纲：注入已完结卷摘要 + 前一卷尾部章节摘要 + 后续各卷 goal（反向约束）
    ↓ 写作中（每章开始时）
  EnsureArcSummaries 懒生成已完结卷的卷摘要（ArcSummary prompt，失败只告警）
    ↓ 上下文换挡
  buildOutlineConstraintsForLang：有摘要的卷压缩为一行卷摘要，
  只有当前卷及未总结卷保留逐章大纲 → 前文上下文从 O(章数) 降为 O(卷数)
    ↓ 无限连载
  POST /api/arcs/append：追加新卷（title/goal/chapter_count）+ 生成章纲，
  自动扩大 config.chapter_count；失败回滚卷条目
```

普通短篇不受影响：`state.Arcs` 为空时所有路径与 v2 行为一致。删除大纲（`DELETE /api/outline`）同时清空 Arcs。

### 大纲反向约束 + 写前一致性检查

防止「后续章节安排的人物/事件提前出现」与「一次性事件（初遇、身份揭示）重复发生」的三层防线（两者是同一根因：写前面章节时看不到后续大纲，事件意外提前发生，后面又按大纲再写一遍）：

1. **事前（写前检查）**：`GenerateChapterAction` 第 1 步调用 `checkOutlineConsistency`（第 1 章跳过），AI 对照前情提要 + 上一章结尾检查本章大纲是否已与实际剧情冲突（如大纲安排初遇但前文已认识），冲突时用 `revised_outline` 最小化替换本章大纲并立即落盘，再开始写正文；检查失败不阻塞（按原大纲继续）
2. **事中（写作约束）**：`buildOutlineConstraintsForLang` 生成「全书章节脉络」块注入写作 prompt——后续 10 章大纲（严禁提前发生/剧透）+ 前文全部章节大纲（一次性事件不得重复发生）
3. **事后（核查闭环）**：事实核查 prompt 注入本章大纲 + 章节脉络，核查范围含「提前引入后续章节事件」「一次性事件重复发生」两项，FAIL 触发最多 3 次自动重写

兜底防线：`ChapterSummary` 模板含【人物动态】条目（出场人物、初次见面、身份揭示等一次性事件），若某事件已意外提前发生，后续章节的前情提要会明确记录，配合「严格承接前情」要求处理为延续而非重新发生。

老项目兼容：prompts 随 `config.json` 持久化，旧模板缺新占位符时 `appendIfMissingPlaceholder` 把约束块（事实核查还含补充核查规则）追加到渲染结果末尾，保证老项目同样生效。

### 章节状态机

```
pending → writing → review → accepted
 ↗
 （修改后回到 review）
```

### 事实核查冲突恢复

事实核查耗尽重试后：章保持 `writing`，`progress.pending_writing_conflict` 落盘，任务结束（非硬失败）。写作页展示冲突面板：

- **去改大纲 / 去改伏笔**：导航到对应页；大纲页可编辑该 `writing` 章大纲（经 `sessionStorage` 聚焦）
- **修改后重试**：`conflict-resolve` `retry` 清空 conflict → 前端再 `POST /api/chapter/generate`
- **保留当前稿进入审核** / **稍后处理**：二者均 `force_review`（`StatusReview` + 清空 conflict），避免留下不可操作的孤儿 `writing`
- **孤儿恢复**：若章为 `writing` 但无 `pending_writing_conflict`（旧版 dismiss / 崩溃窗口），写作页显示恢复条：重新生成或保留稿进入审核

### Agent 输出截断（max_tokens）

`CallAPIStreamMessages` / `CallAPIMessagesSync` 解析 `choices[0].finish_reason`。Agent 循环中若 `finish_reason == "length"` 且响应含未闭合的 `<tool_call>` 或 `parseToolCall` 失败，返回 `agent.output_truncated`（含当前有效 `max_tokens`，Agent 调用下限 8192），**不**补全截断 JSON、**不**用残缺 arguments 执行工具。用户可在配置页增大 `max_tokens` 或缩短指令后，在聊天面板点「重试」重发上一条消息。

### 引用式段落修订

写作页「修改意见」支持框选正文 → 点击「引用到修改意见」，前端将选中片段以 Markdown 引用行（`> ` 前缀）插入意见框。提交修订时：

1. `extractQuotedSentences` 解析引用行与非引用意见
2. `findParagraphsContaining` 在章节正文中定位包含引用句的自然段（`\n\n` 分段，无空行时退化为 `\n`）
3. `reviseChapterSegment` 用 `ChapterSegmentRevision` prompt 只重写匹配段；AI 输出段数须与匹配段一致
4. 定位失败或段数不匹配 → `errSegmentFallback`，日志 `log.chapter_segment_fallback`，回退 `ChapterRevision` 整章修订

### 伏笔生命周期

```
（AI 建议 → 用户确认 / 手动创建 / 助理 create_foreshadow）
  → planted → progressing → resolved
                         ↘ abandoned
```

写作时：`formatActiveForeshadowsForChapterLang` 注入活跃伏笔到 `ChapterWriting` prompt。  
章末：`GenerateChapterAction` / `ReviseChapterAction` / `ReviseSpecificChapterAction` 调用 `syncForeshadowsAfterChapter`（AI 更新状态 + events + resolution），并写入项目目录 `Foreshadows.md` 路线图。  
超期：`BuildForeshadowWarnings` 在日志面板告警（超过预计回收章 3 章以上）。

前端「伏笔」页提供列表、按章节时间线、Markdown 路线图预览；SSE `foreshadow_suggestions` 触发建议确认面板。

### 叙事记忆系统

弥补历史摘要窗口（5 章）之外的叙事细节丢失。每章写作完成后（`GenerateChapterAction` 第 6 步），AI 从正文中提取大纲未体现的关键叙事细节，存入 `Progress.MemoryEntries`。

**数据结构**：`MemoryEntry` 含 `ID`、`Content`（关键细节描述）、`Category`（character/location/item/event/promise/other）、`Chapter`（来源章节号）、`Position`（段落序号，用于自动截取原文片段）。

**Token 上限**：`calcMemoryMaxTokens` 根据全书预估总字数自动计算（`章节数 × 每章字数 / 10`，clamp 到 2000–20000）。超限时 AI 在更新时合并或删除最不重要条目。

**注入机制**：`buildMemoryForLang` 将记忆格式化为 `[第X章] 内容（原文："自动截取片段"）`，注入 `ChapterWriting` 和 `FactCheck` prompt 的 `{{.Memory}}` 占位符。

**同步维护**：`ReviseChapterAction` / `ReviseSpecificChapterAction` 修订章节后删除该章旧记忆并重新提取。

前端「记忆」页（`#memory`）只读展示 `memory_entries`：统计概览、列表/按章节时间线、分类与章节筛选、原文片段预览；数据来自 `GET /api/progress`（无独立 API）。

### 进度持久化（v3 分文件存储）

每个关键步骤后立即调用 `SaveProgress`。v3 起章节正文与元数据分离：

- `progress.json`：项目元数据（phase、标题、章节列表**不含正文**、伏笔、记忆等）
- `chapters/NNNNNN.json`：每章一个文件（num/title/content），`SaveProgress` 内部用 fnv 哈希做脏检查，只重写内容变化的章节，并清理孤儿章节文件
- 内存模型不变：`Progress.Chapters[].Content` 加载后常驻内存，所有业务代码照旧读写
- `GET /api/progress` 返回 `ProgressView`（正文剥离，携带 `word_count` 与 `content_rev`），正文经 `GET /api/chapters/{num}` 按需拉取；前端用 `content_rev` 失效缓存

API 配置保存 `api.json`，故事配置保存 `config.json`。设定保存 `settings.json`。使用原子写入（先写 `.tmp` 再 rename）。

### 结构化设定

`settings.json` 存储 `ProjectSettings`，包含 `Characters`、`Worldview`、`Organizations`、`Relations` 四个数组。这些设定在章节写作时通过 `buildCharacterContextForLang`/`buildWorldviewContextForLang`（`internal/story/inject.go`）注入到 prompt 中。设定也支持 AI 自动生成（`POST /api/settings/ai-generate`）。

### 会话管理

聊天会话存储为项目目录 `sessions/` 下的 JSON 文件。`sessions/index.json` 为会话索引。每个会话文件名为 `{id}.json`。使用 `fsutil.WriteFileAtomic` 保持一致性。

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

## 导入流水线（v3）

```
大纲页空状态 → 「导入已有内容」→ 粘贴全文
  → POST /api/import/split（同步本地切章，无 AI）
      标题正则：第X章/回、Chapter N、序章/楔子/尾声/Prologue/Epilogue；
      无标题文本按 ~6000 字块切分（段落对齐，占位标题）
  → 前端展示切章预览（章号/标题/字数/正文摘录），确认无误
  → POST /api/import/start（异步，任务锁内）：
      0. 切章落盘：全部章节 status=accepted 含正文，Phase="outline"（检查点零）
      1. 元信息分析（ImportMetaAnalysis prompt：开篇节选 + 章节标题表 →
         title/type/core_prompt/synopsis/style/pov；只填 config 空字段，用户已填值优先；
         失败仅告警不阻塞）
      2. 逐章处理：ImportChapterAnalysis prompt（正文截前 6000 字）→ outline + summary，
         每章完成立即 SaveProgress + import.json 游标 +1（断点）
      3. ≥40 章自动分卷（每卷 30 章，尾卷 <10 章并入前卷）→ EnsureArcSummaries 卷摘要
      4. 完成删除 import.json
  → 任务可随时 POST /api/task/stop；再次进入大纲页显示恢复横幅
  → POST /api/import/resume 从断点继续（跳过已有 outline+summary 的章节）
  → 导入完成后「生成后续大纲」（POST /api/outline/generate-continuation）
     或「追加新卷」（POST /api/arcs/append）继续创作
```

断点文件 `import.json`（项目目录）：`{active, total, cursor, meta_done, preamble}`；`GET /api/import/status` 查询。导入要求空项目（已有章节返回 409）。

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
| GET | `/api/version` | 同步 | 获取应用版本号（CI 注入，非 CI 编译为 `dev`） |
| POST | `/api/projects/select` | 同步 | 切换到指定项目 |
| DELETE | `/api/projects/{name}` | 同步 | 删除项目 |
| GET | `/api/config/api` | 同步 | 获取 API 配置 |
| PUT | `/api/config/api` | 同步 | 保存 API 配置 |
| GET | `/api/config` | 同步 | 获取故事配置 |
| PUT | `/api/config` | 同步 | 保存故事配置 |
| GET | `/api/config/pending-changes` | 同步 | 获取 AI 待确认配置变更提案 |
| POST | `/api/config/apply-changes` | 同步 | 采纳选中的 pending 配置变更 |
| DELETE | `/api/config/pending-changes` | 同步 | 清空 pending 配置变更提案 |
| GET | `/api/progress` | 同步 | 获取进度（v3：章节不含正文，含 `word_count`/`content_rev`） |
| GET | `/api/chapters/{num}` | 同步 | 获取单章完整正文（含 `blocks`） |
| PUT | `/api/chapters/{num}/blocks/{id}` | 同步 | 手动编辑单个 block 文本（保持 ID） |
| DELETE | `/api/chapters/{num}/blocks/{id}` | 同步 | 删除单个 block |
| POST | `/api/chapters/{num}/blocks` | 同步 | 在指定 block 后插入新 block（`after_id`=0 表示开头） |
| POST | `/api/chapters/{num}/blocks/{id}/revise` | 异步 | AI 修订单个 block（任务名 `block_revision`） |
| GET | `/api/export/txt` | 同步 | 导出全书 TXT（text/plain） |
| DELETE | `/api/progress` | 同步 | 重置进度（含 `chapters/` 目录） |
| GET | `/api/status` | 同步 | 获取状态摘要 |
| POST | `/api/outline/generate` | 异步 | 生成大纲（存在已确认章节时返回 409 拒绝） |
| POST | `/api/outline/confirm` | 同步 | 确认大纲 |
| POST | `/api/outline/revise` | 异步 | 修订大纲 |
| POST | `/api/outline/generate-continuation` | 异步 | 生成续写大纲 |
| POST | `/api/arcs/skeleton` | 异步 | 生成全书卷级骨架（存在已确认/写作中章节时 409 拒绝，会清空 pending 章节） |
| POST | `/api/arcs/{id}/outline` | 异步 | 为指定卷生成逐章大纲（body 可带 `requirements` 补充要求；卷内存在非 pending 章节时拒绝） |
| POST | `/api/arcs/append` | 异步 | 追加新卷并生成其章纲（`{title?, goal?, chapter_count?}`，默认 20 章；完成后自动扩大 `chapter_count`） |
| POST | `/api/outline/characters/confirm` | 同步 | 批量采纳未登记大纲人物为角色条目 |
| PUT | `/api/outline/{num}` | 同步 | 编辑指定章节大纲（`pending`/`writing`/`review`；`accepted` 拒绝） |
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
| POST | `/api/chapter/conflict-resolve` | 同步 | 处理写作冲突或孤儿 `writing`：`retry`（需有 conflict）；`force_review`/`dismiss`（≡保留稿→`review`，无 conflict 时也对当前 `writing` 前沿章生效） |
| POST | `/api/foreshadows/outline-check` | 异步 | 手动触发伏笔-大纲一致性检查 |
| POST | `/api/chapter/confirm` | 同步 | 确认章节 |
| POST | `/api/chapter/edit` | 同步 | 局部编辑章节正文（`replace_lines`/`replace_text`/`insert_after_line`/`append`） |
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
| DELETE | `/api/chapter` | 同步 | 删除写作前沿章节正文（`DeleteFrontierChapter`） |
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
| POST | `/api/import/split` | 同步 | 本地切章预览（无 AI，不持久化） |
| POST | `/api/import/start` | 异步 | 开始导入流水线（切章落盘 → 元信息 → 逐章分析 → 分卷汇总；仅空项目） |
| POST | `/api/import/resume` | 异步 | 从断点恢复导入 |
| GET | `/api/import/status` | 同步 | 查询导入断点状态（`{active, total, cursor, ...}`） |
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
| `outline_character_suggestions` | `OutlineCharacterSuggestion[]` | 大纲中出现但未在角色管理登记的人物建议 |
| `foreshadow_outline_conflicts` | `ForeshadowOutlineReport` | 伏笔与大纲一致性检查发现冲突 |
| `writing_conflict` | `WritingConflict` | 事实核查多次失败且无法自动调和，等待用户选择处理方向 |
| `settings_reconciled` | `{explanation, changed_fields}` | 设定协调完成 |
| `config_change_proposal` | `ConfigFieldChange[]` | AI 建议修改用户已填配置字段（需用户在配置/大纲页确认采纳） |
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
| `ChapterSegmentRevision` | `chapter_segment_revision` | 引用式段落级修订（修改意见含 `> ` 引用行时，只重写匹配自然段） |
| `ChapterSummary` | `chapter_summary` | 摘要提炼（含【人物动态】一次性事件记录） |
| `FactCheck` | `fact_check` | 事实核查（含提前引入/重复发生检测） |
| `OutlineRevision` | `outline_revision` | 大纲修订 |
| `ForeshadowPlanning` | `foreshadow_planning` | 伏笔规划 |
| `ForeshadowUpdate` | `foreshadow_update` | 伏笔状态更新 |
| `ImportMetaAnalysis` | `import_meta_analysis` | 导入元信息分析（开篇节选 + 章节标题表 → 书名/类型/核心提示词/梗概/风格/视角） |
| `ImportChapterAnalysis` | `import_chapter_analysis` | 导入逐章分析（单章正文 → 章节大纲 + 前情摘要，含【人物动态】一次性事件） |
| `ContinuationOutlineGeneration` | `continuation_outline_generation` | 续写大纲生成 |
| `SettingsReconciliation` | `settings_reconciliation` | 设定协调 |
| `TransitionSmoothing` | `transition_smoothing` | 章节衔接优化（判断 + 最小化重写开头片段，无需修改时输出 NO_CHANGE） |
| `OutlineConsistencyCheck` | `outline_consistency_check` | 写前大纲一致性检查（对照前情提要 + 上一章结尾，冲突时输出最小化修订后的本章大纲） |
| `ForeshadowOutlineConsistency` | `foreshadow_outline_consistency` | 伏笔与完整大纲一致性检查 |
| `OutlineCharacterCheck` | `outline_character_check` | 大纲人物与已登记角色一致性检查 |
| `WritingConflictAnalysis` | `writing_conflict_analysis` | 事实核查多次失败后的根因分析与处理建议 |
| `BookDiagnosis` | `book_diagnosis` | 全书完稿诊断报告（只诊断不改写） |
| `BookConsistencyCheck` | `book_consistency_check` | 全书一致性核查（超长书按卷分段） |
| `BookRoadmap` | `book_roadmap` | 诊断+核查报告 → 结构化工单 JSON |
| `MemoryUpdate` | `memory_update` | 叙事记忆提取与更新（每章完成后提取大纲未体现的关键细节） |
| `ArcSkeleton` | `arc_skeleton` | 全书卷级骨架生成（每卷 title/goal/chapter_count，总章数强制等于配置） |
| `ArcChapterOutline` | `arc_chapter_outline` | 单卷逐章大纲生成（注入卷摘要前情 + 后续卷反向约束 + 用户补充要求） |
| `ArcSummary` | `arc_summary` | 卷摘要压缩（逐章摘要 → 400~800 字卷级前情，保留一次性事件与遗留悬念） |

新增 prompt 模板时需要：(1) 在 `PromptsConfig` 添加字段，(2) 在 `DefaultPrompts` 添加默认值，(3) 在 `applyDefaults` 添加 fallback。

## ChapterWriting 模板占位符

| 占位符 | 来源 | 说明 |
|--------|------|------|
| `{{.Title}}` | `preferUserValue(cfg.Story.Title, state.Title)` | 小说标题（优先用户配置） |
| `{{.ChapterNum}}` | `ch.Num` | 章节编号 |
| `{{.CorePrompt}}` | `state.CorePrompt` | 核心写作提示词 |
| `{{.StorySynopsis}}` | `preferUserValue(cfg.Story.StorySynopsis, state.StorySynopsis)` | 故事梗概（优先用户配置） |
| `{{.HistorySummary}}` | `buildHistorySummaryForLang()` | 最近 5 章摘要 |
| `{{.PreviousEnding}}` | `buildPreviousChapterTailForLang()` | 上一章结尾原文约 800 字（段落对齐，含说明包装；第 1 章或上一章无内容时为空） |
| `{{.ChapterTitle}}` | `ch.Title` | 本章标题 |
| `{{.ChapterOutline}}` | `ch.Outline` | 本章大纲（修订时附加用户修改意见） |
| `{{.WritingStyle}}` | `cfg.Story.WritingStyle` | 写作风格（始终使用当前配置） |
| `{{.WritingPOV}}` | `cfg.Story.WritingPOV` | 叙述视角（如第一人称女主、第三人称限知；老模板缺占位符时由 `appendIfMissingPlaceholder` 追加） |
| `{{.CharacterContext}}` | `buildCharacterContextForLang()` | 结构化角色详情（从 settings 匹配） |
| `{{.WorldviewContext}}` | `buildWorldviewContextForLang()` | 结构化世界观详情（从 settings 匹配） |
| `{{.TargetWords}}` | snapshot | 每章目标字数（prose units：`prose.CountProseUnits`，非 raw rune 数） |
| `{{.TargetWordsMin}}` / `{{.TargetWordsMax}}` | `calcChapterLengthRange` | 可接受正文字数区间（±1000 或 ±15% 取较大者，单位同为 prose units；老模板缺占位符时由 `finalizeChapterWritingPrompt` 追加说明块） |
| `{{.Foreshadows}}` | `formatActiveForeshadowsForChapterLang()` | 活跃伏笔上下文 |
| `{{.Memory}}` | `buildMemoryForLang()` | 叙事记忆（早期章节的关键细节，含自动截取的原文片段；无记忆时为空；老模板缺占位符时追加到 prompt 末尾） |
| `{{.OutlineConstraints}}` | `buildOutlineConstraintsForLang()` | 全书章节脉络反向约束（后续 10 章大纲防提前出现 + 前文大纲防一次性事件重复；无内容时为空；老模板缺占位符时追加到 prompt 末尾） |

## 内置 Skill 文件

| 文件 | ID | 语言 | 分类 | 说明 |
|------|----|------|------|------|
| `internal/story/embeds/skills/humanizer-zh.md` | `humanizer-zh` | zh | polish | 23 条中文 AI 痕迹禁用模式 + 高频短语替换表（top 50） + 口语化/格式规范规则 |
| `internal/story/embeds/skills/story-deslop.md` | `story-deslop` | zh | polish | 6-Gate 中文检测流程 + AI 味检测报告模板 + 真人写作基准表 |
| `internal/story/embeds/skills/writing-craft.md` | `writing-craft` | zh | writing | 章首钩子 7 式 + 章尾钩子 13 式 + 爽点密度 + 节奏控制（中文网文范式） |
| `internal/story/embeds/skills/humanizer-en.md` | `humanizer-en` | en | polish | **本地化非翻译**：针对英文 LLM 输出的 30 条禁用模式（delve/tapestry/em-dash 滥用/said-bookisms/filtering 等）+ 高频替换表 + 风格规则（缩写/Anglo-Saxon 词/具体名词）+ 英文格式规范（US/UK 引号、em-dash 间距、ellipsis） |
| `internal/story/embeds/skills/story-deslop-en.md` | `story-deslop-en` | en | polish | 6-Gate 英文检测：slop-word 密度（per 1000 words）/英文陈词/em-dash & 三联结构/语音差异/emotion-by-body/节奏与开篇结尾，按英文 trade fiction 基线 |
| `internal/story/embeds/skills/writing-craft-en.md` | `writing-craft-en` | en | writing | 章首钩子 7 式 + 章尾钩子 13 式 + Sanderson 的 promise/progress/payoff + Swain scene-and-sequel 节奏 + 对话经济 + 角色 voice + 按场景类型节奏表（英文 trade fiction 范式，非"爽点"框架） |

Skill 文件格式：YAML frontmatter（`---` 分隔，含 `lang: zh|en`，无 `lang` 视为语言无关）+ Markdown body。`LoadAllSkills` 通过 `FilterSkillsByLang` 按 `cfg.Language` 过滤可见 skill。前端通过 `GET /api/skills` 获取列表，`PUT /api/skills/{id}/toggle` 切换启用状态。

**英文 skill 是本地化设计而非中文翻译**：英文 LLM 的 AI 痕迹（delve / tapestry / em-dash 泛滥 / said-bookisms / "in a world where" 等）与中文 LLM 的痕迹（宛如 / 不禁 / 微微 / 缓缓 / 心中暗道 等）不同；写作技法框架也按英文 trade fiction 约定（Sanderson、Swain）而非中文网文的爽点密度。

## 前端架构

前端使用 Vite 5 + Svelte 4 + Tailwind CSS 4 + DaisyUI 5 构建，产物输出到 `frontend/dist/`，通过 `//go:embed frontend/dist` 内嵌到 Go 二进制。主题使用 xianii 暗色主题（定义在 `src/app.css` 的 `@plugin "daisyui/theme"` 块中）。

- **页面**：`config`（配置直接保存 + 角色管理 + 世界观管理 + 组织管理（卡片 + 角色成员勾选）+ 关系管理（卡片 + 源/目标实体下拉，实体覆盖角色/组织/世界观，值编码为 `type:id`））、`outline`（大纲直接操作 + 内联编辑 + 导入续写）、`writing`（写作直接操作 + 定向修订 + 自动确认模式开关 + 伏笔追踪摘要 + 导出 TXT）、`foreshadows`（伏笔 CRUD + AI 建议确认 + 列表/时间线/路线图三视图）、`memory`（叙事记忆只读观测）、`relations`（关系图谱 Canvas）、`skills`（技能管理）
- **状态管理**：Svelte stores（`src/lib/stores.js`），包含 progress、config、settings、taskRunning、taskTokenUsage（任务 token 累计）、autoConfirm（自动确认模式）、foreshadowSuggestions/foreshadowShowSuggestions（AI 伏笔建议待确认）、pendingConfigChanges/showConfigChangePanel（AI 配置变更待确认）等全局状态
- **路由**：hash 路由（`src/lib/router.js`），`currentPage` store + `window.hashchange` 监听
- **API 调用**：`api(method, url, body)` 封装 fetch（`src/lib/api.js`）
- **SSE**：`connectSSE()` 建立 EventSource 连接，14 种事件类型自动更新 stores（`src/lib/sse.js`）；content_chunk/chat_chunk 经 150ms 节流缓冲批量刷入；`chat_message` 的 `task_end` 须始终 `clearChatBuf()`（异步工具子任务仍运行时 `taskCount>0`，否则 reload 后的 messages 与延迟 flush 的 `streaming_text` 重复展示同一段 reply）；`token_usage` 更新 taskTokenUsage，任务运行中 poll `/api/status` 兜底（间隔与 `TaskTokenBadge` 数字线性动画时长共用 `frontend/src/lib/tokenPoll.js` 的 `TOKEN_POLL_INTERVAL_MS`）；任务成功完成以 toast 提示（不弹全屏遮罩）
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

- [`internal/config/config.go`](internal/config/config.go)：`Config.Language` 字段、`DefaultConfigForLang(lang)`、`PromptsConfig.ApplyDefaults(lang)` 按语言回填空字段（`NormalizeLanguage` 在 `internal/i18n`）
- [`internal/config/prompts.go`](internal/config/prompts.go)：`DefaultPromptsZH`
- [`internal/config/prompts_en.go`](internal/config/prompts_en.go)：`DefaultPromptsEN` 全量英文模板（与中文一一对应）
- [`internal/i18n/messages.go`](internal/i18n/messages.go)：`messageCatalog`（`log.*` / `agent.*`）；新增后端日志或 Agent 状态消息时在此加 key，并同步 `frontend/src/lib/i18n/zh.js` + `en.js`（位置占位 `{0}`/`{1}`）
- [`internal/i18n/locale.go`](internal/i18n/locale.go)：`errorCatalog` + `T()`；所有 API 错误走 `Handlers.writeErrorReq(w, r, code, key, args…)`（在 `internal/httpapi/handlers.go`）
- [`internal/sse/logger.go`](internal/sse/logger.go)：`InfoKey`/`SuccessKey`/… 替代硬编码中文；SSE `LogEntry.msg_key` + `msg_args`
- [`internal/agent/agent.go`](internal/agent/agent.go)：工具状态返回用 `agentMsg(ctx, "agent.xxx", …)`；读工具的数据型返回仍按项目语言格式化，不带 key
- [`internal/story/chat.go`](internal/story/chat.go)：`ChatMessage.tool_result_key`/`tool_result_args` 持久化；`AgentStep` 同字段
- [`frontend/src/lib/i18n/index.js`](frontend/src/lib/i18n/index.js)：`formatLogEntry` / `formatToolResult` 按 `uiLocale` 渲染服务端 key
- [`frontend/src/components/ChatPanel.svelte`](frontend/src/components/ChatPanel.svelte)：任务日志与工具结果走 key 化渲染

### 前端关键文件

- [`frontend/src/lib/i18n/zh.js`](frontend/src/lib/i18n/zh.js) / [`frontend/src/lib/i18n/en.js`](frontend/src/lib/i18n/en.js)：UI 文案用 `{name}`；镜像 `log.*`/`agent.*` 服务端 key 用 `{0}`/`{1}`
- [`frontend/src/lib/api.js`](frontend/src/lib/api.js)：所有请求带 `X-UI-Locale`；`writeErrorReq` 已按请求语言返回错误，``translateServerMessage`` 仅 legacy 兜底
- [`frontend/src/lib/sse.js`](frontend/src/lib/sse.js)：`formatLogEntry` / `formatToolResult`
- [`frontend/src/lib/stores.js`](frontend/src/lib/stores.js)：新增 `projectLanguage` writable
- [`frontend/src/App.svelte`](frontend/src/App.svelte)：Header 显示项目语言 badge（ZH/EN）+ 版本号badge + 新版本更新提示（非dev版本检查GitHub releases，点击跳转最新release页面）+ UI 语言切换按钮（中 / EN）；选择/创建项目后自动 `setLocale(project.language)`
- [`frontend/src/pages/Projects.svelte`](frontend/src/pages/Projects.svelte)：新建项目表单名称全宽 + 中文/EN 分段按钮选语言，POST 时携带 `language`；列表项显示语言 badge

### 老项目兼容

- `config.json` 无 `language` 字段 → `NormalizeLanguage` 视为 `"zh"`
- 已有非空 `prompts` 字段 → `applyDefaults(lang)` 仅填空字段，**不**用 EN 模板覆盖
- 已有章节 / 设定不受影响，继续以原语言生成

## 重要约束

1. **零外部依赖**：Go 后端仅使用标准库，不要引入第三方包（前端 npm 依赖不受此限制）
2. **包依赖单向**：`internal/` 各包依赖方向为 `httpapi → agent → story → {llm, config, sse, i18n, prose, fsutil}`，禁止反向依赖；`sse` 保持领域无关（事件负载 `any`）
3. **前端构建**：前端在 `frontend/` 目录中使用 Svelte 组件开发，`npm run build` 产物输出到 `frontend/dist/`，不要拆分构建产物
4. **嵌入式文件**：前端通过 `main.go` 的 `//go:embed frontend/dist` 嵌入，skill 文件通过 `internal/story/skills.go` 的 `//go:embed embeds/skills` 嵌入（目录在 `internal/story/embeds/skills/`），修改后需重新编译
5. **配置文件 gitignore**：`*.json` 被 gitignore，不要提交配置/进度/设定/会话文件
6. **提示词用 `{{.KeyName}}`**：不是 Go `text/template`，是简单字符串替换
7. **异步任务互斥**：同一时间只能有一个 AI 任务运行（`tryStartTask`/`endTask`）
8. **原子写入**：配置和进度文件使用 `fsutil.WriteFileAtomic`（先写 `.tmp` 再 rename）
9. **双语界面**：UI 文案走 `$t('key', {name})`；后端日志/Agent 状态走 `messageCatalog` key + `InfoKey`/`agentMsg`；API 错误走 `writeErrorReq` + `errorCatalog`；新增 key 须同步 `internal/i18n/messages.go`（或 `errorCatalog`）与 `zh.js`/`en.js`
10. **Skill 可选性**：所有 skill 默认禁用，功能性 AI 不注入任何 skill，除非作者显式启用
11. **多语言一致**：新增 prompt 模板必须同时在 `internal/config/prompts.go`（`DefaultPromptsZH`）和 `prompts_en.go`（`DefaultPromptsEN`）补齐；新增注入块文本必须在 `internal/story/inject.go` 处理两种语言；新增内联 system prompt 必须挂到 `internal/i18n/locale.go` 的 `systemPrompts` map

## 修改检查清单

完成代码修改后，必须执行以下检查：

1. `go build ./...` 编译通过，`go test ./...` 全部通过
2. 确认无未使用的 import 或变量（`go vet ./...`）
3. 如果修改了 API 端点，确认 `internal/httpapi/web.go` 中路由已注册
4. 如果新增了 prompt 模板，确认 `internal/config/config.go` 和 `ApplyDefaults` 已更新
5. 如果修改了 SSE 事件，确认前端已添加对应监听
6. 如果新增了 Skill 文件，确认在 `internal/story/embeds/skills/` 目录中，并设置 `lang: zh|en` frontmatter（语言无关的写 `lang: ""` 或省略）
7. 如果新增了 prompt / system prompt / 注入块，确认中英双语都已补齐（见多语言一致约束）
8. 如果新增了前端可见文案，确认 `zh.js` 与 `en.js` 同步加 key
9. **同步更新本 AGENTS.md 文件** + 必要时同步更新 [`README.md`](README.md) 与 [`README.en.md`](README.en.md)
