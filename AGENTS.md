# AGENTS.md — AI 小说生成器项目指南

> 修改代码、配置、前端、提示词或构建流程后，必须同步更新本文件；只记录长期有效的约束与当前架构，不记录单次修复历史或逐函数清单。

## 项目概览

- 单二进制 Go Web 应用；Go 后端只使用标准库，前端产物与内置 Skill 通过 `embed.FS` 嵌入。
- Go `1.25.1`，模块 `showmethestory`；默认端口 `:48090`，可用 `PORT` 覆盖。
- 前端：Vite 5、Svelte 4、Tailwind CSS 4、DaisyUI 5、`@xianii/design-system`。
- 当前项目格式固定为 v4；项目默认保存在程序目录的 `storys/<项目名>/`。
- 项目语言 `zh` / `en` 决定模型提示词、正文和内置 Skill；UI 语言由浏览器独立切换。
- 用户文档为 `README.md` 与 `README.en.md`；许可证为 MIT。

## 常用命令

```bash
task build                 # npm install + 前端构建 + Go 二进制
task build:go              # 只构建 Go；要求 frontend/dist 已存在
task dev                   # 构建并启动后端
task dev:frontend          # Vite 开发服务器 :5173，代理 /api 到 :48090

go build ./...
go test ./...
go vet ./...
cd frontend && npm run build

node frontend/src/lib/projectRestore.check.js
node frontend/src/lib/forceGraphLayout.check.js
```

提交前至少运行 `go build ./...`、`go test ./...`、`go vet ./...`；修改前端时再运行前端构建及受影响的 `.check.js`。

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
- 当前程序只打开 `project_format_version: 4`。v2/v3/未知格式仅可只读探测并提示对应版本，不加载、不迁移、不写回。
- 运行时配置/项目 JSON、`storys/`、`frontend/dist/`、二进制和 `dev.log` 均是本地/生成内容，不提交；已跟踪的 `frontend/package-lock.json` 例外。
- 章节 `Content` 是正文事实源；`Blocks` 从正文派生并保持稳定 ID，块编辑后重建 `Content`。
- 影响正文的请求使用内容版本校验；关联事实受影响时必须显式确认，过期版本返回 409。

## 核心流程

### 异步任务

- AI 端点统一走 `tryStartTask()`，后台执行并 `defer endTask()`；同一时间只允许一个 AI 任务。
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
- 正文、修订、核查、记忆与设定同步复用 `knowledge_retrieval.go` 的本地 BM25 检索；无命中不回退全量知识。
- 检索预算只限制注入的知识上下文，不删除存储中的事实、设定或来源历史。
- 伏笔、摘要、事实核查和大纲一致性检查必须服从明确的结尾意图；通用章尾钩子不能覆盖收尾要求。

### 导入与完稿校订

- 导入先本地切章，再写 accepted 章节并通过 `import.json` 保存断点；元信息和逐章分析按检查点续跑。
- `postprocess.json` 只接受 v1；自动校订保持 block 数量与对应关系，不改变剧情结构，支持逐章撤销。
- 校订报告锚定章节/block；正文变动后刷新锚点。校订不触发写作知识同步。

## LLM、提示词与 Skill

- API 地址通过 `resolveChatCompletionsURL` 统一解析；严格模式只补 `/chat/completions`，普通裸域名补 `/v1/chat/completions`。
- 流式读取使用 `bufio.Reader`；损坏 SSE、缺少 `finish_reason`/`[DONE]` 或提前 EOF 都是错误，半截响应不得进入 JSON 解析。
- 401/403/404 为致命错误；可重试错误沿用现有指数退避。Agent 已收到流片段后失败时不得再拼接同步回退结果。
- 提示词占位符是 `config.RenderPrompt` 的 `{{.Key}}` 字符串替换，不是 `text/template`。
- 新增 prompt 字段时同步更新 `PromptsConfig`、中英默认模板和 `ApplyDefaults`；新增注入块或 system prompt 必须同时提供中英文。
- Skill 包为 `skill.json + SKILL.md`，仅接受安全校验后的 `.md/.txt/.json`；所有 Skill 默认禁用，并按项目语言、`applies_to` 和动作类别过滤。
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
- SSE 的 content/chat chunk 使用现有缓冲节流；任务结束清空聊天缓冲，状态恢复以 `/api/status` 为准。

## 测试与精简规则

- 测试按领域文件组织；同一函数的输入变体优先表驱动，跨流程测试保留独立名称。不要仅为减少文件数合并无关测试，Go 已按 package 统一编译。
- 测试 helper 先用标准库和现有 helper；只有多个测试共享且能明显减少重复时才新增 helper。
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
