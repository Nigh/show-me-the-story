# AGENTS.md — AI 小说写手项目指南

> 修改代码、配置、前端、提示词或构建流程后，必须同步更新本文件；只记录长期有效的约束与当前架构，不记录单次修复历史或逐函数清单。

## 项目概览

- 单二进制 Go Web 应用，完整应用体积不到 5 MB；Go 后端只使用标准库，前端产物与内置 Skill 通过 `embed.FS` 嵌入。
- Go `1.25.1`，模块 `showmethestory`；默认端口 `:48090`，可用 `PORT` 覆盖。
- 前端：Vite 5、Svelte 4、Tailwind CSS 4、DaisyUI 5、`@xianii/design-system`；Playwright 仅使用系统 Google Chrome 生成 README 截图。
- 当前项目格式固定为 v4；项目默认保存在程序目录的 `storys/<项目名>/`。
- 项目语言 `zh` / `en` 决定模型提示词、正文和内置 Skill；UI 语言由浏览器独立切换。
- 默认英文入口为 `README.md`，中文入口为 `README.zh.md`；语言导航保持独立的首行链接列表，便于继续增加语言。详细流程维护于 `docs/guide.zh.md` 与 `docs/guide.en.md`，按创作顺序提供双语步骤与故障排查；许可证为 MIT。

## 常用命令

```bash
task build                 # npm install + 前端构建 + Go 二进制
task build:go              # 只构建 Go；要求 frontend/dist 已存在
task dev                   # 构建并启动后端
task dev:frontend          # Vite 开发服务器 :5173，代理 /api 到 :48090
task screenshots           # 用固定离线样例重建 README 截图

go build ./...
go test ./...
go vet ./...
cd frontend && npm run build

node frontend/src/lib/projectRestore.check.js
node frontend/src/lib/forceGraphLayout.check.js
cd frontend && npm run check # 所有前端回归检查
```

提交前至少运行 `go build ./...`、`go test ./...`、`go vet ./...`；修改前端时再运行前端构建及受影响的 `.check.js`。

PR 与 main/v4 推送通过同一检查工作流在 Linux/Windows 原生执行构建、测试及 vet；Linux 额外运行 race 检查。发布复用该工作流，检查成功后才生成发布产物。

## 架构边界

```text
main.go
└── internal/httpapi
    ├── internal/agent
    │   └── internal/story
    └── internal/story
        ├── internal/llm
        ├── internal/config
        ├── internal/sse
        ├── internal/i18n
        ├── internal/prose
        └── internal/fsutil
```

- `main.go`：程序目录、API 配置、开发日志、嵌入前端和服务装配。
- `internal/httpapi/`：路由、请求校验、项目管理、异步任务互斥、SSE 和本地化错误。
- `internal/agent/`：助理循环、工具解析与执行、危险操作确认。
- `internal/story/`：大纲、写作、事实与设定、伏笔、导入、Skill、会话、校订等领域逻辑。
- `internal/llm/`：OpenAI 兼容客户端、流式完整性、重试、token 统计、JSON 提取。
- `internal/config/`：API/项目配置和中英默认提示词。
- `internal/sse/`、`i18n/`、`prose/`、`fsutil/`、`devlog/`：领域无关基础能力。
- `frontend/src/`：Svelte 页面、组件、stores、API/SSE/i18n；生产产物仅在 `frontend/dist/`。

依赖必须保持单向：`httpapi → agent → story → 基础包`。`sse` 的事件负载保持 `any`，不得反向依赖领域包。不要恢复已废弃的根目录 `static/` 单页实现。

## 持久化与兼容边界

- v4 的 `progress.json` 只存元数据，正文存于 `chapters/NNNNNN.json`；API 通过 `ProgressView` 返回无正文视图和派生字段。
- 配置、进度、设定、会话及校订状态写入必须复用 `fsutil.WriteFileAtomic` 或现有领域保存函数；先提交元数据，再清理孤儿章节文件。
- 正文与进度保存由进程内存储锁串行执行；覆盖前以 `progress.json.rollback` 保存本次涉及文件的旧字节，删除回滚日志后才视为提交并清理孤儿文件。加载、保存或重置前先恢复未完成事务；恢复失败保留日志、阻止操作并返回结构化 `SaveError`。该日志不提供历史版本，也不支持多个程序同时写同一项目。
- 章节读取、JSON 或身份校验失败必须阻止项目加载并保留章号、路径和原因；仅完全未写的 pending 章节允许文件缺失，不得把已有正文的读取失败当作空章节。
- 当前程序只打开 `project_format_version: 4`。v2/v3/未知格式仅可只读探测并提示对应版本，不加载、不迁移、不写回。
- 运行时配置/项目 JSON、`storys/`、`frontend/dist/`、二进制和 `dev.log` 均是本地/生成内容，不提交；已跟踪的 `frontend/package-lock.json` 例外。
- 章节 `Content` 是正文事实源；`Blocks` 从正文派生并保持稳定 ID，块编辑后重建 `Content`。
- 影响正文的请求使用内容版本校验；关联事实受影响时必须显式确认，过期版本返回 409。
- 项目列表支持完整项目 ZIP 备份及恢复为新项目；不包含全局 API 配置或用户 Skill。备份/恢复仅在 AI 空闲时执行，请求写操作与备份快照串行；恢复先在隐藏临时目录校验路径、大小、JSON、v4 和正文完整性，再发布新目录，禁止覆盖已有项目。上限为 ZIP 256 MiB、解压总量 512 MiB、单文件 64 MiB、20000 个条目。

## 核心流程

### 异步任务

- AI 端点统一走 `tryStartTask()`，后台执行并 `defer endTask()`；同一时间只允许一个 AI 任务。
- 章纲编辑及伏笔确认附带的一致性检查同样在修改前预留任务锁，沿用 `taskCtx`、Skill 激活与任务事件；失败或取消不得发布新报告，保存失败必须保留存储诊断，所有提前返回释放预留。
- `taskCtx` 承载取消与任务 token 统计；SSE 提供日志、流式片段、任务状态和领域刷新事件。
- `endTask` 在释放互斥前同步本任务改动且已跟踪的正文知识；校订任务显式跳过该流程。
- 存储失败必须保留 `fsutil.SaveError` 的结构化诊断，不得用普通成功 Toast 覆盖。

### 批次大纲与结尾

- 首批和后续批次统一使用 `POST /api/outline/generate-continuation` 与 `OutlineBatchRequest`。
- 每批需要 `chapter_count`（1–36）和 `outline_synopsis`；可选长期方向与结尾意图。
- 默认 `append` 接在最大章号后；`replace_last` 只替换末尾完整、全部 pending、无正文的批次，并要求确认。
- 预定完结批次之后继续规划需要 `confirm_continue=true`；真正完结仍由用户手动操作。
- 模型输出必须通过章节数量、连续编号和章纲长度校验；失败不得部分提交内存或磁盘状态。
- 首批且配置/进度书名都为空时允许模型推断书名；后续批次不得覆盖已有书名。

### 写作、事实与设定

- 章节状态为 `pending → writing → review → accepted`；失败草稿不得作为正式知识来源。
- 生成、确认、人工编辑、AI 修订、润色和衔接优化均进入知识同步；只处理已跟踪章节，不扫描旧章节补齐。
- 事实使用稳定 ID 与 `MemoryReference{chapter, block_id, quote, content_rev, stale}`；模型只能复用检索上下文中的相同 ID/原文，新事实使用 `id=0`。
- 事实或设定同步的 JSON/身份/证据校验失败可纠正重试一次；最终失败保留原数据和待同步状态，不提交部分结果、不消耗 ID。
- 设定只从 accepted 正文自动同步。无冲突的明确演变可应用；与作者设定冲突或证据不足的变化进入待确认建议。
- 世界观 `knowledge` 分类表示作者维护的小说知识规则，可以完全虚构；正文同步对已有知识规则的修改必须进入待确认建议。写作页可保存知识并修订选中章节，也可选择已有世界观条目；保存与修订分步执行，修订失败保留设定。
- 章节修订可携带 `worldview_ids`：后端读取完整条目并显式注入作者修改意见，不依赖检索、不静默裁剪，仍受模型总输入预算限制；需要当前正文版本与关联事实影响确认。前端按所选设定修订使用指定章节流程，不自动改其他章纲或正文；自定义修订模板缺少 UserFeedback 占位符时仍补入修改意见。
- 正文、修订、核查、记忆与设定同步复用 `knowledge_retrieval.go` 的本地 BM25 检索；无命中不回退全量知识。
- 检索预算只限制注入的知识上下文，不删除存储中的事实、设定或来源历史。
- 长篇上下文保留最近 20 章原始摘要；超过 40 个已确认章节后，下一次写作、批次规划或规划复盘按每 20 章生成带来源哈希的持久化检查点，每 10 个同级节点继续向上压缩。旧章修改或删除后按需重建；模型失败使用带 degraded 标记的限长本地摘要，下次使用重试并刷新受影响父节点。哈希版本变化会重建旧检查点；取消不得提交新的检查点，来源哈希不占用摘要正文预算。
- 正文只注入有界的近期历史、分层远期摘要、BM25 相关旧章纲和未来 10 章；批次规划、规划复盘、大纲修订、设定协调及伏笔检查同样必须复用有界历史，不得恢复全量历史拼接。
- 伏笔、摘要、事实核查和大纲一致性检查必须服从明确的结尾意图；通用章尾钩子不能覆盖收尾要求。

### 导入与完稿校订

- 导入先本地切章，再写 accepted 章节并通过 `import.json` 保存断点；元信息和逐章分析按检查点续跑。
- `postprocess.json` 只接受 v1；自动校订保持 block 数量与对应关系，不改变剧情结构，支持逐章撤销。
- 校订报告锚定章节/block；正文变动后刷新锚点。校订不触发写作知识同步。
- 完稿后创建的续写项目以 `inherited` 标记前作章节与伏笔；前作章节保留为规划上下文但不计入新项目章节进度，伏笔使用独立快照并在界面标明继承来源语义。

## LLM、提示词与 Skill

- API 地址通过 `resolveChatCompletionsURL` 统一解析；严格模式只补 `/chat/completions`，普通裸域名补 `/v1/chat/completions`。
- 流式读取使用 `bufio.Reader`；损坏 SSE、缺少 `finish_reason`/`[DONE]` 或提前 EOF 都是错误，半截响应不得进入 JSON 解析。
- 401/403/404 为致命错误；可重试错误沿用现有指数退避。Agent 已收到流片段后失败时不得再拼接同步回退结果。
- 每个模型请求以 `ContextBudgetTokens - MaxTokens - max(4096, 5% 上下文窗口)` 作为保守估算的输入上限；模型端点能报告更小的真实窗口时自动向下收紧配置，最终预检超限属于不可重试错误。
- 提示词占位符是 `config.RenderPrompt` 的 `{{.Key}}` 字符串替换，不是 `text/template`。
- 新增 prompt 字段时同步更新 `PromptsConfig`、中英默认模板和 `ApplyDefaults`；新增注入块或 system prompt 必须同时提供中英文。
- Skill 包必须包含 schema v1 的 `skill.json` 及其声明的 Markdown 入口（通常为 `SKILL.md`），仅接受安全校验后的 `.md/.txt/.json`；所有 Skill 默认禁用，并按项目语言、`applies_to` 和动作类别过滤。
- 内置 Skill 位于 `internal/story/embeds/skills/`，修改后需要重新编译。

## 前端约定

- 启动立即尝试恢复服务端当前项目，不等待版本检查；SSE 恢复或任务开始也会触发恢复。任务运行时禁止返回项目列表。
- 所有请求复用 `api` / `apiFetch`，携带当前 UI 语言并统一解析本地化错误；下载失败不得提示成功。
- UI 文案走 `$t('key', params)`；后端日志/Agent 结果使用 key + args；新增可见文案必须同步 `zh.js` 与 `en.js`。
- 项目语言不可变；选择/创建项目时 UI 语言可跟随项目初始化，之后允许独立切换。
- 使用 `@xianii/design-system` token。正文默认 16px；`text-sm` 用于紧凑控件，`text-xs` 仅用于元数据。
- 全局平面风格：`--depth: 0`，无阴影；主要操作实心语义色，普通操作 `btn-outline`，弱操作 `btn-ghost`，危险操作 `btn-error btn-outline`。
- `tabs-box` 是统一描边分段控件；活动项主色填充。正文段落 hover 淡底、点击选中，文字操作栏仅在选中后显示。
- 中间工作区与助理约 2:1，助理宽 18–28rem；中间列必须 `min-w-0`，写作正文网格使用 `minmax(0, 1fr)`。
- 写作章节列表宽 345px；章节正文按需加载，重复点击当前章节必须能够重试失败请求。
- 事实/设定面板位于正文卡片内并默认折叠；摘要常驻，事实标记可展开并定位来源段落。
- 核心操作必须有直接页面按钮，不依赖聊天；破坏性操作用 `ConfirmModal`，基础可访问性不得为精简让步。
- SSE 的 content/chat chunk 使用现有缓冲节流；任务结束清空聊天缓冲，重连、结束事件和运行中轮询均以 `/api/status` 校准状态，不使用本地事件计数判定空闲；跨越更新任务事件或请求的旧响应不得覆盖新状态。

## 测试与精简规则

- 测试按领域文件组织；同一函数的输入变体优先表驱动，跨流程测试保留独立名称。不要仅为减少文件数合并无关测试，Go 已按 package 统一编译。
- 测试 helper 先用标准库和现有 helper；只有多个测试共享且能明显减少重复时才新增 helper。
- README 截图由 `frontend/scripts/screenshots.mjs` 使用系统 Google Chrome 与固定离线样例按 README 语言生成 WebP 到 `docs/screenshots/<语言>/`；工作流只在前端、截图文件或截图工作流变更的 PR 中运行，并允许手动触发。
- 读取源码执行的前端回归脚本必须兼容 LF/CRLF，不依赖平台或 Git 换行配置。
- 非平凡分支、解析、存储安全或兼容边界的改动必须留下最小回归测试；纯删除死代码无需新增测试。
- 不新增单实现接口、工厂、无消费者配置、转发 wrapper 或“以后可能用”的兼容层。
- 优先删除死代码和重复文档；API/SSE/prompt 的完整枚举以路由、结构体和源码为准，不在本文件复制一份。

## 修改检查清单

1. 搜索所有调用者，修共享根因而不是单个症状。
2. 保持包依赖、任务互斥、原子保存、版本校验和双语边界。
3. API 变更核对 `internal/httpapi/web.go`；SSE 变更核对 `frontend/src/lib/sse.js`。
4. prompt/system prompt/注入块/前端文案同步中英文。
5. 执行本文件“常用命令”中的适用检查。
6. 同步更新本文件；用户行为变化时再同步中英文 README。

## graphify

项目知识图谱位于 `graphify-out/`（已 gitignore）。代码库问题优先运行：

```bash
graphify query "<问题>"
graphify path "<A>" "<B>"
graphify explain "<概念>"
```

代码修改后运行 `graphify update .`；图谱生成文件变脏不构成跳过理由。
