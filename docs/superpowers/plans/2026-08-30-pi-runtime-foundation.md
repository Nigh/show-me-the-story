# Pi Runtime Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a macOS-local, independently configured Pi runtime beside the existing Go application, expose provider/model/auth and per-story session foundations through private APIs, and prove old projects remain readable without switching any production novel-generation path.

**Architecture:** A supervised Node.js process embeds `@earendil-works/pi-coding-agent` and owns Pi credentials, provider/model discovery, and Pi session objects under `<program-dir>/pi-data`. Go remains the public HTTP server and canonical novel-domain owner; it talks to Node over a private Unix-domain socket. During this foundation stage the existing direct-API generation implementation remains active, Pi sessions have all tools/resources/extensions disabled, and no Pi endpoint may prompt a model or write story data.

**Tech Stack:** Go 1.25.1 standard library, Node.js >= 22.19.0, TypeScript 5.9, Vitest 4, `@earendil-works/pi-coding-agent` 0.83.0, `@earendil-works/pi-ai` 0.83.0, Svelte frontend unchanged in this stage.

**Spec:** [2026-08-30-pi-web-host-redesign.md](../specs/2026-08-30-pi-web-host-redesign.md)

## Global Constraints

- Implement only specification Phase 0 and Phase 1. Do not change the Svelte frontend, generation buttons, chat behavior, prompt construction, review workflow, plugin installation, plugin execution, or canonical story-write paths.
- Keep the existing Go direct-API pipeline working throughout this stage. Pi startup failure degrades only the new `/api/pi/*` endpoints and must not prevent the current application from starting.
- Store every Pi-owned file under `<program-dir>/pi-data`; never read or write `~/.pi`, `~/.pi/agent`, or the existing project `sessions/` folders.
- Pin Pi packages exactly at `0.83.0`. Use normal `node_modules` resolution because Pi needs package-adjacent metadata/assets; do not bundle the runtime into one JavaScript file.
- Require Node.js `>=22.19.0`. This phase targets the confirmed macOS local environment and uses a Unix-domain socket with owner-only permissions.
- Tests must use temporary directories, fake credentials, fake runtime ports, and local sockets only. They must not need internet access, a real provider account, or a model response.
- Treat Go project data as canonical. The Node process may create only `pi-data/**`; it must not open or mutate `storys/**`, `api.json`, `config.json`, `progress.json`, chapter Markdown, or post-processing files.
- Legacy `api.json` import is copy-only. Normalize its URL in Go with the existing `resolveAPIBase`, merge the provider into Pi's `models.json`, persist the key through Pi auth, and leave `api.json` untouched.
- Never return API keys, OAuth tokens, auth prompt secret answers, or raw credential payloads to the browser or logs.
- Protect every new `/api/pi/*` route with a local/same-origin guard because the existing generic CORS middleware allows `*`: require a loopback `RemoteAddr`; when `Origin` is present, require an HTTP(S) origin whose host exactly equals `r.Host`. Requests without `Origin` remain available to local CLI/tests. Do not change legacy-route CORS behavior in this phase.
- Preserve all unrelated dirty-worktree changes. Before editing, capture `git status --short`; for files already dirty (`.gitignore`, `AGENTS.md`, `README.md`, `README.en.md`, `handlers.go`, `locale.go`, and `web.go` in the planning snapshot), inspect the pre-existing diff and use `git add -p` to stage only foundation hunks. Never stage a whole overlapping file. Before every commit, run `git diff --cached --name-only` and `git diff --cached` to confirm no user hunk entered the index.
- Update `AGENTS.md` together with the new runtime/build contract, as required by the repository instructions.

## Planned File Structure

```text
show-me-the-story/
├── pi-runtime/
│   ├── package.json                 # pinned runtime dependencies and scripts
│   ├── package-lock.json            # reproducible Node dependency graph
│   ├── tsconfig.json                # NodeNext strict TypeScript build
│   ├── README.md                    # foundation runtime contract and local commands
│   └── src/
│       ├── files.ts                 # private directories and atomic JSON writes
│       ├── files.test.ts
│       ├── paths.ts                 # independent pi-data and per-story hashed paths
│       ├── paths.test.ts
│       ├── protocol.ts              # secret-free request/response DTOs
│       ├── models.ts                # Pi ModelRuntime adapter and legacy import
│       ├── models.test.ts
│       ├── login-broker.ts          # asynchronous browser/Pi auth handshake
│       ├── login-broker.test.ts
│       ├── sessions.ts              # one no-tools/no-resources Pi session per story
│       ├── sessions.test.ts
│       ├── server.ts                # internal HTTP router over Unix socket
│       ├── server.test.ts
│       └── index.ts                 # validated CLI startup and signal handling
├── project_compat_test.go           # Phase 0 legacy-project characterization
├── canonical_write_compat_test.go   # explicit inventory of canonical write primitives
├── pi_runtime_types.go              # Go-side stable DTOs and interface
├── pi_runtime_client.go             # private Unix-socket HTTP client
├── pi_runtime_client_test.go
├── pi_runtime_supervisor.go         # Node version check, lifecycle, restart, degrade
├── pi_runtime_supervisor_test.go
├── pi_handlers.go                   # public Web API adapter; no production AI calls
├── pi_handlers_test.go
├── main.go                          # best-effort supervisor lifecycle
├── web.go                           # route registration and dependency injection
├── handlers.go                      # optional PiRuntime dependency only
├── Taskfile.yml                     # install/test/build the runtime
├── .gitignore                       # track runtime manifests; ignore build/data
├── README.md
├── README.en.md
└── AGENTS.md
```

---

### Task 1: Lock Down Legacy Project Read Compatibility

**Files:**

- Create: `project_compat_test.go`
- Create: `canonical_write_compat_test.go`
- Read for behavior only: `config.go`, `settings.go`, `state.go`, `web.go`, `handlers.go`
- Test: `project_compat_test.go`

- [ ] **Step 1: Capture the dirty-worktree boundary and baseline**

Run:

```bash
git status --short
go version
node --version
go test ./...
```

Expected: record the pre-existing dirty paths before editing; Go is 1.25.1-compatible, Node is at least 22.19.0, and the current Go suite passes. If the baseline fails, invoke `superpowers:systematic-debugging`, record the pre-existing failure, and do not attribute it to Pi work.

- [ ] **Step 2: Add a temporary legacy-project fixture helper**

Write JSON fixtures at runtime so the repository-wide `*.json` ignore rule is irrelevant. Capture hashes only after the first explicit load, because current loaders may add backward-compatible defaults during that initial load.

```go
type canonicalSnapshot map[string][32]byte

func requireMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil { t.Fatal(err) }
}

func requireWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil { t.Fatal(err) }
}

func writeLegacyFixture(t *testing.T, root string) string {
	t.Helper()
	projectDir := filepath.Join(root, "storys", "旧项目")
	requireMkdirAll(t, projectDir)
	requireWriteFile(t, filepath.Join(projectDir, "config.json"), []byte(`{
  "story": {
    "type":"悬疑","title":"旧书","chapter_count":12,
    "target_words_per_chapter":2400,"writing_style":"现实主义",
    "writing_pov":"第三人称限知"
  },
  "prompts":{"outline_generation":"保留的旧提示词"}
}`))
	requireWriteFile(t, filepath.Join(projectDir, "progress.json"), []byte(`{
  "phase":"writing","current_chapter_index":1,
  "chapters":[
    {"num":3,"title":"已确认章","content":"旧正文","status":"accepted"},
    {"num":4,"title":"审核章","content":"待审正文","status":"review"}
  ]
}`))
	requireWriteFile(t, filepath.Join(projectDir, "settings.json"), []byte(`{"characters":[],"worldview":[],"organizations":[],"relations":[]}`))
	requireWriteFile(t, filepath.Join(projectDir, "Chapter_03.md"), []byte("# 第三章\n\n旧正文\n"))
	return projectDir
}
```

- [ ] **Step 3: Characterize legacy load and project-selection behavior**

Add tests with these exact assertions:

```go
func TestLegacyProjectLoadsWithoutLosingProgress(t *testing.T) {
	root := t.TempDir()
	projectDir := writeLegacyFixture(t, root)

	cfg, err := LoadConfig(filepath.Join(projectDir, "config.json"))
	if err != nil { t.Fatal(err) }
	state, err := LoadProgress(filepath.Join(projectDir, "progress.json"))
	if err != nil { t.Fatal(err) }

	if cfg.Story.Title != "旧书" || cfg.Language != "zh" || cfg.Story.ChapterCount != 12 {
		t.Fatalf("legacy config changed: %#v", cfg)
	}
	if state.Phase != "writing" || state.CurrentChapterIndex != 1 {
		t.Fatalf("legacy progress changed: %#v", state)
	}
	if len(state.Chapters) != 2 || state.Chapters[0].Status != "accepted" || state.Chapters[1].Status != "review" {
		t.Fatalf("review states changed: %#v", state.Chapters)
	}
}
```

For `TestProjectReadEndpointsDoNotMutateCanonicalFiles`, create `snapshotCanonicalFiles(t, projectDir)` using `os.ReadFile` plus `sha256.Sum256`, call `switchProject` once so legacy prompt defaults are normalized, then take the baseline. Exercise `GetProjectCurrent`, `GetConfig`, and `GetProgress` with `httptest.NewRequest`/`httptest.NewRecorder`; require status 200 and compare `config.json`, `progress.json`, `settings.json`, and `Chapter_03.md` digests byte-for-byte afterward.

- [ ] **Step 4: Characterize switching, import/review/confirmation, and cancellation**

Add the following focused tests to `project_compat_test.go`:

- `TestProjectSwitchKeepsEachProjectIsolated`: create two fixtures with different titles/progress, switch A → B → A, verify the in-memory title/current index each time, and verify both projects' canonical snapshots are unchanged after each switch.
- `TestLegacyReviewConfirmTransitionsPersist`: begin from an outline-phase fixture with pending chapters; call `ConfirmOutlineAction` and assert phase `writing`; persist a representative generated chapter in `review` state plus `Chapter_01.md`; call `ConfirmChapterAction`, assert the chapter becomes `accepted` and `CurrentChapterIndex` advances. Reload `progress.json` after every transition and compare it to memory. The previously planned outline/chapter import characterization is omitted because those handlers were part of the pre-existing uncommitted work that the user explicitly deleted before implementation; this foundation does not recreate them.
- `TestTaskCancellationLeavesCanonicalCheckpointUntouched`: build a selected handler, capture its canonical snapshot, install a cancellable running task context under `taskMu`, invoke `PostTaskStop`, require status 200 and `ctx.Err() == context.Canceled`, then require the snapshot is unchanged. Call `endTask` during cleanup so the test leaves no running state.

These tests call current action/handler APIs and must not introduce a fake Pi dependency.

- [ ] **Step 5: Lock the complete direct filesystem-write surface**

In `canonical_write_compat_test.go`, add `TestCanonicalWritePrimitivesRemainAllowlisted`. Parse every non-test root-package Go file with `go/parser`. Record the unique calls to `os.WriteFile`, `os.Remove`, `os.RemoveAll`, `os.Rename`, `writeFileAtomic`, `writeFile`, `deleteFile`, and `renameFile` as `file:function:callee`, then compare the sorted set with a checked-in literal allowlist for the current tree. The initial allowlist must include only the current direct write sites in:

```text
agent.go:getBuiltinTools:deleteFile
chat.go:saveChatSessions:writeFileAtomic
chat.go:SaveChatSession:writeFileAtomic
chat.go:DeleteChatSession:deleteFile
config.go:saveAPIConfig:writeFileAtomic
config.go:saveConfig:writeFileAtomic
config_guard.go:SavePendingConfigChanges:writeFileAtomic
config_guard.go:SavePendingConfigChanges:deleteFile
filesys.go:writeFileImpl:os.WriteFile
filesys.go:deleteFileImpl:os.Remove
filesys.go:renameFileImpl:os.Rename
foreshadow.go:SaveForeshadowRoadmap:os.WriteFile
handlers.go:PutAPIConfig:writeFileAtomic
handlers.go:PutConfig:writeFileAtomic
handlers.go:PostChatSession:writeFileAtomic
handlers.go:DeleteProgress:deleteFile
handlers.go:DeleteChaptersFrom:deleteFile
handlers.go:GetChatSessions:deleteFile
handlers.go:writeFileAtomic:writeFile
handlers.go:writeFileAtomic:renameFile
handlers.go:writeFileAtomic:deleteFile
postprocess.go:SavePostProcess:writeFileAtomic
settings.go:SaveProjectSettings:writeFileAtomic
state.go:SaveProgress:writeFileAtomic
state.go:SaveChapterMarkdown:os.WriteFile
web.go:DeleteProject:os.RemoveAll
writing_delete.go:clearChapterContentAt:deleteFile
```

If the AST reveals a current direct site missing from this list, inspect it and add its exact `file:function:callee` entry before proceeding; do not wildcard an entire file or package. This test makes every future canonical write an explicit review event and will also prove the new `pi_*.go` files do not write story data.

- [ ] **Step 6: Run the Phase 0 baseline**

Run:

```bash
go test ./... -run 'TestLegacyProject|TestProjectReadEndpointsDoNotMutateCanonicalFiles|TestProjectSwitchKeepsEachProjectIsolated|TestLegacyReviewConfirmTransitionsPersist|TestTaskCancellationLeavesCanonicalCheckpointUntouched|TestCanonicalWritePrimitivesRemainAllowlisted' -count=1
```

Expected: PASS. These are characterization tests, so this task intentionally starts green; any failure is an existing compatibility problem that must be understood before Pi work begins.

- [ ] **Step 7: Commit the compatibility boundary**

```bash
git add project_compat_test.go canonical_write_compat_test.go
git commit -m "test: lock legacy project read compatibility"
```

---

### Task 2: Scaffold the Independent Pi Runtime and Private Paths

**Files:**

- Create: `pi-runtime/package.json`
- Create: `pi-runtime/package-lock.json`
- Create: `pi-runtime/tsconfig.json`
- Create: `pi-runtime/src/files.ts`
- Create: `pi-runtime/src/files.test.ts`
- Create: `pi-runtime/src/paths.ts`
- Create: `pi-runtime/src/paths.test.ts`
- Modify: `.gitignore`

- [ ] **Step 1: Add failing path-isolation and file-mode tests**

```ts
import { chmod, mkdtemp, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, sep } from "node:path";
import { describe, expect, it } from "vitest";
import { ensureRuntimePaths, projectPaths, resolveRuntimePaths } from "./paths.js";

describe("Pi data isolation", () => {
  it("keeps every path under the explicit data directory", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-"));
    const paths = resolveRuntimePaths(root);
    await ensureRuntimePaths(paths);
    const project = projectPaths(paths, "../越界/故事");

    for (const path of Object.values({ ...paths, ...project })) {
      expect(path === root || path.startsWith(root + sep)).toBe(true);
    }
    expect(paths.agentDir.includes(".pi")).toBe(false);
    expect((await stat(paths.root)).mode & 0o777).toBe(0o700);
  });

  it("repairs an overly broad root mode", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-"));
    await chmod(root, 0o755);
    await ensureRuntimePaths(resolveRuntimePaths(root));
    expect((await stat(root)).mode & 0o777).toBe(0o700);
  });
});
```

Run:

```bash
cd pi-runtime && npm test -- src/paths.test.ts
```

Expected: FAIL because the package and path module do not exist yet.

- [ ] **Step 2: Create the pinned Node package and strict compiler config**

Use this package contract, then run `npm install` once to generate and commit the lockfile:

```json
{
  "name": "show-me-the-story-pi-runtime",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "engines": { "node": ">=22.19.0" },
  "scripts": {
    "build": "tsc -p tsconfig.json",
    "test": "vitest run",
    "typecheck": "tsc -p tsconfig.json --noEmit",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "@earendil-works/pi-ai": "0.83.0",
    "@earendil-works/pi-coding-agent": "0.83.0"
  },
  "devDependencies": {
    "@types/node": "24.12.4",
    "typescript": "5.9.3",
    "vitest": "4.1.9"
  }
}
```

`tsconfig.json` must use `module`/`moduleResolution: "NodeNext"`, `target: "ES2023"`, `strict: true`, `rootDir: "src"`, `outDir: "dist"`, declaration files, source maps, and `noUncheckedIndexedAccess: true`.

- [ ] **Step 3: Implement private paths and atomic JSON writes**

Use an opaque project id so story names can never become filesystem paths:

```ts
export interface RuntimePaths {
  root: string;
  agentDir: string;
  authPath: string;
  modelsPath: string;
  modelsStorePath: string;
  projectsDir: string;
  stagingDir: string;
  rollbackDir: string;
  runDir: string;
  socketPath: string;
}

export function projectId(name: string): string {
  return createHash("sha256").update(name.normalize("NFC")).digest("hex").slice(0, 16);
}

export function projectPaths(paths: RuntimePaths, name: string) {
  const root = join(paths.projectsDir, projectId(name));
  const workspace = join(root, "workspace");
  return {
    projectRoot: root,
    workspace,
    projectPiDir: join(workspace, ".pi"),
    sessionsDir: join(root, "sessions"),
    pluginDataDir: join(root, "plugin-data"),
    scratchDir: join(root, "scratch"),
  };
}
```

`resolveRuntimePaths(dataDir)` must call `resolve(dataDir)` and derive every field from it, including `socketPath = join(runDir, "pi.sock")`. `ensureRuntimePaths` must create/chmod private global directories to `0700`. Add `ensureProjectPaths` to create `workspace/.pi`, `sessions`, `plugin-data`, and `scratch` with mode `0700`; call it from the session factory before opening Pi. Add `readJsonIfExists<T>` and `writeJsonAtomic(path, value, mode = 0o600)` in `files.ts`; the atomic writer must create a same-directory temporary file, `fsync`, rename, chmod, and remove only its own temporary file on failure.

- [ ] **Step 4: Make runtime manifests trackable and generated state ignored**

Add these rules without removing unrelated ignore entries:

```gitignore
!pi-runtime/package.json
!pi-runtime/package-lock.json
!pi-runtime/tsconfig.json
pi-runtime/node_modules/
pi-runtime/dist/
pi-data/
```

- [ ] **Step 5: Run the runtime foundation checks**

```bash
cd pi-runtime
npm install
npm test -- src/paths.test.ts src/files.test.ts
npm run typecheck
```

Expected: all tests PASS, typecheck exits 0, and `package-lock.json` records Pi 0.83.0 exactly.

- [ ] **Step 6: Commit the isolated runtime scaffold**

```bash
git add pi-runtime/package.json pi-runtime/package-lock.json pi-runtime/tsconfig.json pi-runtime/src/files.ts pi-runtime/src/files.test.ts pi-runtime/src/paths.ts pi-runtime/src/paths.test.ts
git add -p .gitignore
git diff --cached --name-only
git diff --cached
git commit -m "feat: scaffold isolated pi runtime"
```

---

### Task 3: Add the Provider/Model Catalog and Copy-Only Legacy Import

**Files:**

- Create: `pi-runtime/src/protocol.ts`
- Create: `pi-runtime/src/models.ts`
- Create: `pi-runtime/src/models.test.ts`
- Test: `pi-runtime/src/models.test.ts`

- [ ] **Step 1: Define the secret-free wire contract**

Put snake-case JSON field names directly in the TypeScript DTOs so the Go and Node layers share one stable protocol:

```ts
export type AuthType = "api_key" | "oauth";

export interface ModelSummary {
  id: string;
  name: string;
  provider_id: string;
  reasoning: boolean;
  context_window: number;
  max_tokens: number;
}

export interface ProviderSummary {
  id: string;
  name: string;
  auth_types: AuthType[];
  configured: boolean;
  models: ModelSummary[];
}

export interface CredentialSummary {
  provider_id: string;
  type: AuthType;
}

export interface LegacyAPIImport {
  base_url: string;
  model: string;
  api_key?: string;
  max_tokens?: number;
}

export interface LegacyImportResult {
  provider_id: "show-me-the-story-legacy";
  model_id: string;
  credential_imported: boolean;
  available: boolean;
  made_default: boolean;
}
```

Do not add a token/key field to any summary or result type.

- [ ] **Step 2: Write failing catalog and import tests with a fake Pi port**

Cover all of these cases:

- provider summaries include `api_key` and/or `oauth` only when `provider.auth.apiKey?.login` and/or `provider.auth.oauth?.login` exists;
- models are grouped under their provider and credentials contain only provider id/type;
- importing `https://example.test/v1`, model `legacy-model`, and key `secret-value` creates/merges provider id `show-me-the-story-legacy` with API `openai-completions` and `authHeader: true`;
- an unrelated custom provider already present in `models.json` survives the merge;
- the API key is absent from `models.json`, return values, and serialized test snapshots;
- `runtime.getAvailable(providerId)` must contain the imported model before default provider/model are set and flushed;
- an import without a key does not call Pi login and does not become default unless Pi already reports that model available;
- an empty model or non-HTTP(S) normalized URL is rejected.

Run:

```bash
cd pi-runtime && npm test -- src/models.test.ts
```

Expected: FAIL because `ModelService` does not exist.

- [ ] **Step 3: Wrap Pi's `ModelRuntime` behind a narrow testable port**

The production constructor must use the independent paths and disable network model discovery during startup:

```ts
const runtime = await ModelRuntime.create({
  authPath: paths.authPath,
  modelsPath: paths.modelsPath,
  modelsStorePath: paths.modelsStorePath,
  allowModelNetwork: false,
});
```

Expose only these methods from `ModelService`:

```ts
export interface ModelCatalog {
  listProviders(): Promise<ProviderSummary[]>;
  listCredentials(): Promise<CredentialSummary[]>;
  importLegacy(input: LegacyAPIImport): Promise<LegacyImportResult>;
  logout(providerId: string): Promise<void>;
}
```

`listProviders` must obtain live data from `getProviders()`, `getModels(providerId)`, `checkAuth(providerId)`, and `listCredentials()` rather than duplicating Pi's provider registry. A provider is `configured` only when `await checkAuth(provider.id)` returns a value; auth choices are derived from the presence of each auth method's `login` function.

- [ ] **Step 4: Implement the legacy provider merge and credential persistence**

Merge this provider into the existing `models.json` object and preserve all other keys/providers:

```ts
const legacyProvider = {
  name: "Imported show-me-the-story API",
  baseUrl: input.base_url,
  api: "openai-completions",
  authHeader: true,
  models: [{
    id: input.model,
    name: input.model,
    ...(input.max_tokens && input.max_tokens > 0 ? { maxTokens: input.max_tokens } : {}),
  }],
};
```

Do not infer `contextWindow` from the old `ContextBudgetTokens`; those values have different meanings. Atomically write mode `0600`, call `runtime.refresh()`, and persist a supplied key only through:

```ts
await runtime.login("show-me-the-story-legacy", "api_key", {
  prompt: async (prompt) => {
    if (prompt.type !== "secret") throw new Error("legacy import expected a secret prompt");
    return input.api_key!;
  },
  notify: () => undefined,
});
```

Then call `runtime.getAvailable(providerId)` and look up the exact imported model id. Only when present may the service call `settings.setDefaultModelAndProvider(providerId, input.model)` and `await settings.flush()`. Return `available` and `made_default` explicitly; a model that is not auth-available remains imported but does not silently replace the current default. Never place the key in an exception, event, or log message.

- [ ] **Step 5: Run model-service checks**

```bash
cd pi-runtime
npm test -- src/models.test.ts
npm run typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit provider/model support**

```bash
git add pi-runtime/src/protocol.ts pi-runtime/src/models.ts pi-runtime/src/models.test.ts
git commit -m "feat: add pi provider and model catalog"
```

---

### Task 4: Bridge Pi API-Key and OAuth Login Without Leaking Secrets

**Files:**

- Create: `pi-runtime/src/login-broker.ts`
- Create: `pi-runtime/src/login-broker.test.ts`
- Modify: `pi-runtime/src/protocol.ts`
- Test: `pi-runtime/src/login-broker.test.ts`

- [ ] **Step 1: Add the asynchronous login protocol**

```ts
export interface AuthPromptView {
  id: string;
  type: "text" | "secret" | "select" | "manual_code";
  message: string;
  options?: Array<{ id: string; label: string; description?: string }>;
}

export interface AuthEventView {
  cursor: number;
  type: "info" | "auth_url" | "device_code" | "progress";
  message?: string;
  url?: string;
  code?: string;
}

export interface LoginSnapshot {
  id: string;
  provider_id: string;
  auth_type: AuthType;
  state: "running" | "waiting" | "succeeded" | "failed" | "cancelled";
  prompt?: AuthPromptView;
  events: AuthEventView[];
  next_cursor: number;
  error?: string;
}

export interface LoginStartRequest {
  provider_id: string;
  auth_type: AuthType;
}

export interface LoginResponse {
  prompt_id: string;
  value: string;
}

export interface LogoutRequest {
  provider_id: string;
}
```

The snapshot deliberately contains prompt metadata but never the submitted answer or credential.

- [ ] **Step 2: Write failing broker tests**

Use a fake `login(providerId, type, interaction)` implementation that emits an auth URL, asks for a secret, waits for the browser response, and then resolves. Verify:

- `start` returns immediately with a random login id;
- `snapshot(id, 0)` exposes ordered cursor events and the pending prompt;
- `respond` unblocks Pi login and clears the prompt;
- `JSON.stringify(snapshot)` never contains the submitted `secret-value`;
- invalid prompt ids and duplicate responses fail;
- `cancel` aborts the Pi interaction and produces `cancelled`;
- `sweepExpired` removes terminal sessions after ten minutes using an injected clock;
- Pi failures become a short sanitized error and do not include prompt answers.

Run:

```bash
cd pi-runtime && npm test -- src/login-broker.test.ts
```

Expected: FAIL because `LoginBroker` does not exist.

- [ ] **Step 3: Implement a bounded in-memory login broker**

Use `crypto.randomUUID()`, one `AbortController` per login, monotonically increasing event cursors, and a single deferred prompt response. The public API must be exactly:

```ts
export class LoginBroker {
  constructor(runtime: Pick<ModelRuntime, "login">, clock: () => number = Date.now);
  start(providerId: string, authType: AuthType): LoginSnapshot;
  snapshot(id: string, afterCursor: number): LoginSnapshot;
  respond(id: string, promptId: string, value: string): void;
  cancel(id: string): void;
  sweepExpired(): void;
  dispose(): void;
}
```

Pass Pi the exact interaction shape:

```ts
const interaction: AuthInteraction = {
  signal: controller.signal,
  prompt: (prompt) => waitForBrowserResponse(login, prompt),
  notify: (event) => appendSanitizedEvent(login, event),
};
```

Map Pi events without inventing field names: `auth_url.url` → `url`, `device_code.userCode` → `code`, and `device_code.verificationUri` → `url`; retain human instructions/messages only after the 4-KiB bound. Honor both the login controller's signal and an individual `AuthPrompt.signal`. Limit each login to 200 events, reject responses above 64 KiB, and never log the `value` argument. `dispose()` must abort all running logins and clear their pending prompt promises.

- [ ] **Step 4: Run auth-broker checks**

```bash
cd pi-runtime
npm test -- src/login-broker.test.ts
npm run typecheck
```

Expected: PASS.

- [ ] **Step 5: Commit the auth bridge**

```bash
git add pi-runtime/src/protocol.ts pi-runtime/src/login-broker.ts pi-runtime/src/login-broker.test.ts
git commit -m "feat: bridge pi provider authentication"
```

---

### Task 5: Create Persistent Per-Story Pi Sessions With Capabilities Disabled

**Files:**

- Create: `pi-runtime/src/sessions.ts`
- Create: `pi-runtime/src/sessions.test.ts`
- Modify: `pi-runtime/src/protocol.ts`
- Test: `pi-runtime/src/sessions.test.ts`

- [ ] **Step 1: Add the session summary and a fake-factory test seam**

```ts
export interface SessionSummary {
  project_id: string;
  session_id: string;
  message_count: number;
  restored: boolean;
}

export interface SessionSelectRequest {
  project_name: string;
}

export interface SessionHandle {
  summary(restored: boolean): SessionSummary;
  dispose(): void;
}

export interface StorySessionFactory {
  open(projectName: string): Promise<{ handle: SessionHandle; restored: boolean }>;
}
```

- [ ] **Step 2: Write failing registry tests**

Verify that selecting the same Unicode-normalized story name resolves the same project id, selecting another story disposes the prior handle, `current()` is secret-free, and `dispose()` clears the registry. Add a production-factory spy test that asserts all resource categories and all tools are disabled.

Run:

```bash
cd pi-runtime && npm test -- src/sessions.test.ts
```

Expected: FAIL because the registry and production factory do not exist.

- [ ] **Step 3: Implement the production session factory**

For each story, use only the hashed `pi-data/projects/<id>` paths from Task 2:

```ts
await ensureProjectPaths(project);
const settings = SettingsManager.create(project.workspace, paths.agentDir, {
  projectTrusted: true,
});
const loader = new DefaultResourceLoader({
  cwd: project.workspace,
  agentDir: paths.agentDir,
  settingsManager: settings,
  noExtensions: true,
  noSkills: true,
  noPromptTemplates: true,
  noThemes: true,
  noContextFiles: true,
});
await loader.reload();

const existing = await SessionManager.list(project.workspace, project.sessionsDir);
const sessionManager = existing.length > 0
  ? SessionManager.continueRecent(project.workspace, project.sessionsDir)
  : SessionManager.create(project.workspace, project.sessionsDir);
const { session } = await createAgentSession({
  cwd: project.workspace,
  agentDir: paths.agentDir,
  modelRuntime,
  settingsManager: settings,
  resourceLoader: loader,
  sessionManager,
  noTools: "all",
});
```

Do not call `session.prompt`, register application tools, load extensions, or inspect the Go story directory. The workspace exists only to give Pi a stable per-story session identity.

- [ ] **Step 4: Implement the one-active-session registry**

```ts
export class StorySessionRegistry {
  constructor(private readonly factory: StorySessionFactory) {}
  async select(projectName: string): Promise<SessionSummary>;
  current(): SessionSummary | undefined;
  dispose(): void;
}
```

Serialize `select` calls with a promise queue so two browser requests cannot leave two live sessions. Dispose the old handle only after the replacement opens successfully.

- [ ] **Step 5: Run session checks**

```bash
cd pi-runtime
npm test -- src/sessions.test.ts
npm run typecheck
```

Expected: PASS, with no provider/network access.

- [ ] **Step 6: Commit session persistence**

```bash
git add pi-runtime/src/protocol.ts pi-runtime/src/sessions.ts pi-runtime/src/sessions.test.ts
git commit -m "feat: add isolated pi story sessions"
```

---

### Task 6: Expose the Node Runtime Only Through a Private Unix Socket

**Files:**

- Create: `pi-runtime/src/server.ts`
- Create: `pi-runtime/src/server.test.ts`
- Create: `pi-runtime/src/index.ts`
- Modify: `pi-runtime/src/protocol.ts`
- Test: `pi-runtime/src/server.test.ts`

- [ ] **Step 1: Define the internal HTTP surface in a router test**

The internal server must expose only:

```text
GET    /health
GET    /v1/providers
GET    /v1/credentials
POST   /v1/auth/logins
GET    /v1/auth/logins/{id}?after={cursor}
POST   /v1/auth/logins/{id}/responses
DELETE /v1/auth/logins/{id}
POST   /v1/auth/logout
POST   /v1/legacy-import
POST   /v1/sessions/select
GET    /v1/sessions/current
```

Write tests against fake model/login/session services for every route, plus method-not-allowed, unknown-route, malformed JSON, body-over-1-MiB, and service-error cases. Assert every response has `application/json`, error bodies are `{ "error": { "code", "message" } }`, and serialized bodies never contain the fake key/token.

Run:

```bash
cd pi-runtime && npm test -- src/server.test.ts
```

Expected: FAIL because `createRuntimeServer` does not exist.

- [ ] **Step 2: Implement the dependency-injected server**

```ts
export interface RuntimeServices {
  models: ModelCatalog;
  logins: LoginBroker;
  sessions: StorySessionRegistry;
}

export function createRuntimeServer(services: RuntimeServices): http.Server {
  return http.createServer((request, response) => {
    void routeRequest(services, request, response).catch((error) => {
      writeInternalError(response, sanitizeError(error));
    });
  });
}
```

Read request bodies with a streaming 1-MiB limit. Validate required strings and integer cursors at the boundary. Do not log request bodies. `/health` returns only `{ "status": "ok", "protocol_version": 1, "pi_version": "0.83.0" }`.

- [ ] **Step 3: Implement validated CLI startup and socket ownership**

`index.ts` must require both `--data-dir <absolute-path>` and `--socket <absolute-path>`, then:

1. resolve and ensure the independent runtime paths;
2. reject a socket path outside `paths.runDir`;
3. remove the exact pre-existing path only when `lstat().isSocket()`; reject a regular file;
4. construct `ModelRuntime`, global `SettingsManager`, `ModelService`, `LoginBroker`, and `StorySessionRegistry`;
5. call `server.listen(socketPath)` and `chmod(socketPath, 0o600)`;
6. on `SIGTERM`/`SIGINT`, stop accepting requests, cancel logins, dispose the session, close the server, and unlink that exact socket.

No startup path may refer to the user's home directory.

- [ ] **Step 4: Test a real Unix-socket lifecycle**

Add one test that starts the dependency-injected server on a temporary Unix socket, sends `/health` through `http.request({ socketPath })`, verifies mode `0600`, closes it, and verifies the socket file is gone. This test still uses fake Pi services and no network.

- [ ] **Step 5: Run Node runtime verification**

```bash
cd pi-runtime
npm test
npm run typecheck
npm run build
```

Expected: PASS, and `dist/index.js` exists.

- [ ] **Step 6: Commit the private runtime API**

```bash
git add pi-runtime/src/protocol.ts pi-runtime/src/server.ts pi-runtime/src/server.test.ts pi-runtime/src/index.ts
git commit -m "feat: serve pi runtime over private socket"
```

---

### Task 7: Add the Go Unix-Socket Client and Stable Runtime Interface

**Files:**

- Create: `pi_runtime_types.go`
- Create: `pi_runtime_client.go`
- Create: `pi_runtime_client_test.go`
- Test: `pi_runtime_client_test.go`

- [ ] **Step 1: Define Go DTOs and the injectable interface**

Mirror `protocol.ts` with explicit JSON tags and no secret field on response types:

```go
type PiRuntime interface {
	Health(context.Context) (PiHealth, error)
	Providers(context.Context) ([]PiProvider, error)
	Credentials(context.Context) ([]PiCredential, error)
	StartLogin(context.Context, PiLoginStartRequest) (PiLoginSnapshot, error)
	LoginStatus(context.Context, string, int) (PiLoginSnapshot, error)
	RespondLogin(context.Context, string, PiLoginResponse) error
	CancelLogin(context.Context, string) error
	Logout(context.Context, string) error
	ImportLegacy(context.Context, PiLegacyImportRequest) (PiLegacyImportResult, error)
	SelectProject(context.Context, string) (PiSessionSummary, error)
	CurrentSession(context.Context) (*PiSessionSummary, error)
	Close() error
}

type PiLegacyImportRequest struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

type PiLoginStartRequest struct {
	ProviderID string `json:"provider_id"`
	AuthType   string `json:"auth_type"`
}

type PiLoginResponse struct {
	PromptID string `json:"prompt_id"`
	Value    string `json:"value"`
}

type PiHealth struct {
	Status          string `json:"status"`
	ProtocolVersion int    `json:"protocol_version"`
	PiVersion       string `json:"pi_version"`
}
```

Keep `APIKey` request-only. `PiProvider`, `PiCredential`, `PiLoginSnapshot`, `PiLegacyImportResult`, and `PiSessionSummary` must not contain it.

- [ ] **Step 2: Write failing HTTP-contract tests**

Use `httptest.NewServer` and an internal constructor that accepts an `*http.Client` plus base URL. Verify exact methods/paths/query/body for every interface method, JSON decoding, `204 No Content`, a 2-MiB response cap, context cancellation, malformed JSON, and the typed error fields from non-2xx responses.

Run:

```bash
go test ./... -run 'TestPiRuntimeClient' -count=1
```

Expected: FAIL because the client does not exist.

- [ ] **Step 3: Implement the Unix transport and bounded JSON helper**

```go
func NewPiRuntimeClient(socketPath string) PiRuntime {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &piRuntimeClient{
		baseURL: "http://pi-runtime",
		http: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}
}
```

Centralize request construction in `doJSON`: marshal once, set `Content-Type`, use `io.LimitReader(response.Body, 2<<20+1)`, reject oversized bodies, decode structured runtime errors, and never format the request body into an error.

- [ ] **Step 4: Run Go client checks**

```bash
gofmt -w pi_runtime_types.go pi_runtime_client.go pi_runtime_client_test.go
go test ./... -run 'TestPiRuntimeClient' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the client**

```bash
git add pi_runtime_types.go pi_runtime_client.go pi_runtime_client_test.go
git commit -m "feat: add pi runtime unix client"
```

---

### Task 8: Supervise Node Without Making Existing Startup Depend on Pi

**Files:**

- Create: `pi_runtime_supervisor.go`
- Create: `pi_runtime_supervisor_test.go`
- Test: `pi_runtime_supervisor_test.go`

- [ ] **Step 1: Write failing version, lifecycle, and degradation tests**

Cover:

- `v22.19.0`, `v22.20.1`, and `v23.0.0` are accepted; `v22.18.9`, malformed output, and command failure are rejected;
- the child receives `dist/index.js --data-dir <progDir>/pi-data --socket <progDir>/pi-data/run/pi.sock`;
- readiness polls `/health` before reporting available;
- after a successful `SelectProject`, the supervisor remembers only that project name in memory and replays `SelectProject` on a replacement client after a crash, restoring the persisted Pi session before reporting healthy;
- unexpected exit restarts with bounded delays `250ms, 500ms, 1s, 2s, 5s` and resets after 30 seconds healthy;
- `Close` sends `SIGTERM`, waits up to five seconds, and then kills only the owned child;
- missing Node, missing entry point, or readiness timeout yields an unavailable implementation while the caller can continue;
- child stdout/stderr is drained with a fixed memory bound but never forwarded to the application logger or browser; lifecycle logs contain only exit status and predefined reason codes.

Inject command creation, clock/timers, and runtime-client creation so tests use the Go test helper process rather than a real Node installation.

Run:

```bash
go test ./... -run 'TestPiRuntimeSupervisor|TestParseNodeVersion' -count=1
```

Expected: FAIL because the supervisor does not exist.

- [ ] **Step 2: Define explicit supervisor options and status**

```go
type PiRuntimeOptions struct {
	NodePath   string
	EntryPath  string
	DataDir    string
	SocketPath string
	Logger     *LogBroadcaster
	Command    func(context.Context, string, ...string) *exec.Cmd
	NewClient  func(string) PiRuntime
}

type PiRuntimeStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Node      string `json:"node_version,omitempty"`
	Pi        string `json:"pi_version,omitempty"`
}
```

The supervisor must implement `PiRuntime` by forwarding to the current healthy client and expose `Status() PiRuntimeStatus` as a concurrency-safe local snapshot. When unavailable, every runtime method returns a sentinel `ErrPiRuntimeUnavailable` without panicking.

- [ ] **Step 3: Implement validation, readiness, restart, and shutdown**

Resolve defaults from `progDir`, not the home directory:

```go
options.EntryPath = filepath.Join(progDir, "pi-runtime", "dist", "index.js")
options.DataDir = filepath.Join(progDir, "pi-data")
options.SocketPath = filepath.Join(options.DataDir, "run", "pi.sock")
```

Check `runtime.GOOS == "darwin"`, `node --version >= 22.19.0`, and the entry file before starting. Use one mutex-protected child/client pair, one monitor goroutine, a cancellable supervisor context, and the fixed bounded restart sequence. Readiness must have a ten-second total deadline and 100ms polls. Override the forwarding `SelectProject` method to remember the name only after Node returns success; after restart health succeeds, replay that selection and require its restore to succeed before marking the replacement healthy. Drain child output into an in-memory ring capped at 32 KiB solely to avoid blocking the process, then discard it on both success and failure; log only lifecycle state, exit code/signal, and predefined reason codes. Never log child text or HTTP bodies.

- [ ] **Step 4: Run supervisor checks and the Go race detector**

```bash
gofmt -w pi_runtime_supervisor.go pi_runtime_supervisor_test.go
go test ./... -run 'TestPiRuntimeSupervisor|TestParseNodeVersion' -count=1
go test -race ./... -run 'TestPiRuntimeSupervisor' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit process supervision**

```bash
git add pi_runtime_supervisor.go pi_runtime_supervisor_test.go
git commit -m "feat: supervise local pi runtime"
```

---

### Task 9: Wire Foundation Pi APIs Into Go Without Cutting Over AI Calls

**Files:**

- Create: `pi_handlers.go`
- Create: `pi_handlers_test.go`
- Modify: `handlers.go:17-70`
- Modify: `web.go:24-43`
- Modify: `main.go:40-81`
- Modify if new localized errors are needed: `locale.go`
- Test: `pi_handlers_test.go`, `project_compat_test.go`

- [ ] **Step 1: Write failing handler tests with a recording fake runtime**

Test these public routes and expected status codes:

```text
GET    /api/pi/status                         200 even when unavailable
GET    /api/pi/providers                      200 or 503
GET    /api/pi/credentials                    200 or 503
POST   /api/pi/auth/login                     202 or 400/503
GET    /api/pi/auth/login/{id}?after={cursor} 200 or 400/404/503
POST   /api/pi/auth/login/{id}/respond        204 or 400/404/503
DELETE /api/pi/auth/login/{id}                204 or 404/503
POST   /api/pi/auth/logout                    204 or 400/503
POST   /api/pi/import-legacy-api/preview      200 or 400
POST   /api/pi/import-legacy-api              200 or 400/503
POST   /api/pi/session/select                 200 or 409/503
GET    /api/pi/session/current                200 or 204/503
```

Required assertions:

- preview calls the existing `resolveAPIBase` and returns provider/model/base URL/max tokens but no API key;
- import forwards the in-memory legacy API key to the private runtime, but the HTTP response and test logger do not contain it;
- import does not modify or remove `api.json`;
- session selection rejects when no Go project is selected and otherwise forwards only the current project name;
- provider/auth/session routes do not alter any project file;
- unavailable Pi returns 503 on capability routes while existing `/api/config/api` and project reads still work.
- a non-loopback remote address or mismatched `Origin` receives 403 before any runtime method is called; loopback requests with no origin or an origin matching `r.Host` proceed.

Run:

```bash
go test ./... -run 'TestPiHandler|TestPiRoutes' -count=1
```

Expected: FAIL because the handlers/routes are absent.

- [ ] **Step 2: Inject Pi without changing existing `NewHandlers` call sites**

Add `piRuntime PiRuntime` to the existing `Handlers` struct immediately after `version`, plus this explicit setter:

```go
func (h *Handlers) SetPiRuntime(runtime PiRuntime) {
	h.piRuntime = runtime
}
```

Keep the existing `NewHandlers(apiCfg, apiCfgPath, logger, progDir, version)` signature so current tests and non-Pi construction remain source-compatible.

- [ ] **Step 3: Implement strict JSON adapters in `pi_handlers.go`**

Use `json.Decoder.DisallowUnknownFields()`, a 1-MiB request limit, the request context, and existing JSON/error helpers. For preview/import:

```go
func (h *Handlers) legacyImportRequest() (PiLegacyImportRequest, error) {
	baseURL := resolveAPIBase(h.apiCfg.BaseURL, h.apiCfg.URLStrict)
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return PiLegacyImportRequest{}, errors.New("legacy API base URL must be HTTP(S)")
	}
	if strings.TrimSpace(h.apiCfg.Model) == "" {
		return PiLegacyImportRequest{}, errors.New("legacy API model is required")
	}
	return PiLegacyImportRequest{
		BaseURL: baseURL,
		Model: h.apiCfg.Model,
		APIKey: h.apiCfg.APIKey,
		MaxTokens: h.apiCfg.MaxTokens,
	}, nil
}
```

Preview must copy that value into a separate response DTO that omits `APIKey`. Map `ErrPiRuntimeUnavailable` to 503, validation errors to 400, missing login ids to 404, and no selected project to 409. Do not echo private runtime error details that may include provider responses.

`GetPiStatus` should type-assert the runtime to `interface{ Status() PiRuntimeStatus }` and return that snapshot. For a plain fake/client without this optional method, derive `{available:true}` from successful `Health` or `{available:false, reason:"unavailable"}` from failure. This is the only Pi route that stays 200 while unavailable.

Add `localPiOnly(next http.HandlerFunc) http.HandlerFunc` in the same file. Parse `r.RemoteAddr` with `net.SplitHostPort`, require `net.ParseIP(host).IsLoopback()`, and, when the `Origin` header is non-empty, parse it with `url.Parse` and require `origin.Host == r.Host` plus scheme `http` or `https`. Return a fixed localized 403 without inspecting the request body.

- [ ] **Step 4: Register only the foundation routes**

Add the route list immediately after the existing API-config routes in `web.go`, wrapping every handler with `localPiOnly`. Do not modify any outline/chapter/settings/chat handler registration and do not add frontend calls.

- [ ] **Step 5: Start the supervisor best-effort from `main.go`**

Construct the supervisor after the logger, pass it into `startWebServer`, and defer `Close`. Change `startWebServer` to accept the optional `PiRuntime` and call `h.SetPiRuntime(piRuntime)` before route registration. Startup must continue with a clear local log message if Pi is unavailable.

Pi project-session selection remains explicit through `/api/pi/session/select` in this stage; do not couple it to existing `PostProjectSelect` until the generation cutover phase.

- [ ] **Step 6: Run handler, compatibility, and race checks**

```bash
gofmt -w main.go web.go handlers.go pi_handlers.go pi_handlers_test.go
go test ./... -run 'TestPiHandler|TestPiRoutes|TestLegacyProject|TestProjectReadEndpointsDoNotMutateCanonicalFiles' -count=1
go test -race ./... -run 'TestPiHandler|TestPiRuntimeSupervisor' -count=1
```

Expected: PASS. Existing generation tests must not require Pi.

- [ ] **Step 7: Commit Go integration**

```bash
git add main.go pi_handlers.go pi_handlers_test.go
git add -p web.go handlers.go locale.go
git diff --cached --name-only
git diff --cached
git commit -m "feat: expose pi runtime foundation api"
```

If `locale.go` needed no change, omit it from `git add`.

---

### Task 10: Integrate Builds, Document the Boundary, and Verify End to End

**Files:**

- Create: `pi-runtime/README.md`
- Modify: `Taskfile.yml`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `AGENTS.md`
- Test: all Go and Node tests

- [ ] **Step 1: Add explicit Pi install/test/build tasks**

Extend `Taskfile.yml` with commands equivalent to:

```yaml
  pi:install:
    dir: pi-runtime
    cmds:
      - npm ci

  pi:test:
    dir: pi-runtime
    cmds:
      - npm test
      - npm run typecheck

  pi:build:
    deps: [pi:install]
    dir: pi-runtime
    cmds:
      - npm run build
```

Make the normal local `build` depend on both the current frontend build and `pi:build` before compiling Go. Extend `clean` to remove only `pi-runtime/dist`; never remove `pi-data` because it contains credentials and sessions.

- [ ] **Step 2: Document the foundation-stage runtime accurately**

In Chinese and English READMEs, document:

- Node.js >=22.19.0 and `task build`/manual equivalent;
- the independent `<program-dir>/pi-data` directory and explicit statement that `~/.pi` is not used;
- API key and OAuth/subscription login support through the new foundation APIs, with the Web settings UI explicitly deferred to the dual-workspace/provider UI plan;
- legacy `api.json` preview/import as copy-only;
- graceful degradation when Pi is absent;
- the important limitation that current novel generation still uses the legacy path in this foundation stage;
- source-checkout startup expectations. Do not claim release artifacts package Node/Pi yet.

In `pi-runtime/README.md`, list the private CLI arguments, internal socket protocol, data ownership, supported Node version, security invariants, and test commands. Mark the internal HTTP API as non-public and versioned at protocol 1.

- [ ] **Step 3: Update repository guidance**

Add to `AGENTS.md`:

- `pi-runtime/` ownership and commands;
- exact Node and Pi versions;
- Go ↔ Node Unix-socket boundary;
- secret-redaction rule;
- prohibition on reading `~/.pi` or writing `storys/**` from Node;
- requirement to run both Node and Go suites when the boundary changes.

- [ ] **Step 4: Run the complete automated verification**

```bash
cd pi-runtime && npm ci && npm test && npm run typecheck && npm run build
cd ..
go test ./...
go test -race ./...
task build
git diff --check
```

Expected: every command exits 0. `task build` produces the existing Go binary plus `pi-runtime/dist`, and no test needs a network connection or real credential.

- [ ] **Step 5: Run a real-process local smoke test**

Use temporary Pi data so developer credentials are never touched:

```bash
pi_smoke_dir="$(mktemp -d)"
node pi-runtime/dist/index.js \
  --data-dir "$pi_smoke_dir/pi-data" \
  --socket "$pi_smoke_dir/pi-data/run/pi.sock" &
pi_smoke_pid=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do
  test -S "$pi_smoke_dir/pi-data/run/pi.sock" && break
  sleep 0.1
done
curl --silent --show-error --unix-socket "$pi_smoke_dir/pi-data/run/pi.sock" \
  http://pi-runtime/health
kill -TERM "$pi_smoke_pid"
wait "$pi_smoke_pid"
test ! -e "$pi_smoke_dir/pi-data/run/pi.sock"
```

Expected health body:

```json
{"status":"ok","protocol_version":1,"pi_version":"0.83.0"}
```

After it exits, move the explicitly captured temporary directory to Trash or remove only that exact `pi_smoke_dir` after confirming it begins with the system temporary directory. Do not use a broad recursive target.

- [ ] **Step 6: Audit the stage boundary manually**

Run:

```bash
git diff --name-only 795aa6d..HEAD
rg -n 'session\.prompt|prompt\(' pi-runtime/src
rg -n '\.pi/agent|~/.pi|homedir\(' pi-runtime main.go pi_runtime_*.go
rg -n 'storys|chapter_.*\.md|progress\.json|settings\.json' pi-runtime/src
```

Expected:

- no frontend file appears in the change list;
- no production Pi prompt call exists;
- no home Pi path exists;
- Node code contains no canonical story-data path;
- generation/chat handlers have no Pi dependency;
- `.superpowers/brainstorm/**` is untracked and absent from every commit.

- [ ] **Step 7: Commit build and documentation integration**

```bash
git add Taskfile.yml pi-runtime/README.md
git add -p README.md README.en.md AGENTS.md
git diff --cached --name-only
git diff --cached
git commit -m "docs: document pi runtime foundation"
```

---

## Foundation Acceptance Checklist

- [ ] An existing project loads with its phase, current chapter, accepted/review states, configuration, settings, and chapter Markdown unchanged by read-only operations.
- [ ] Pi reads/writes only `<program-dir>/pi-data`, with private directory/file permissions and no `~/.pi` fallback.
- [ ] Provider/model lists come from the foundation `ModelRuntime`; the same catalog interface is ready to expose extension-registered providers when approved extension loading is added in the plugin phase.
- [ ] API-key and OAuth/subscription auth handshakes work through the asynchronous broker without secrets entering browser responses or logs.
- [ ] Existing `api.json` can be previewed and copied into Pi; unrelated custom providers survive and the source file remains unchanged.
- [ ] A per-story Pi session can be created/restored, but it has no tools, extensions, skills, templates, themes, context files, prompts, or canonical story access.
- [ ] Go reaches Node only over an owner-only Unix socket and validates/limits every request and response.
- [ ] Missing/incompatible Node or runtime failure does not stop the existing Web app or legacy generation pipeline.
- [ ] No production AI call has moved to Pi, no Svelte file has changed, and no plugin manager has been implemented prematurely.
- [ ] Node tests/typecheck/build, Go tests/race checks, `task build`, smoke test, and `git diff --check` all pass.

## Explicitly Deferred to Later Plans

- Canonical Go domain tools (`novel.read_*`, proposal/commit/reject) and the dual workspace UI.
- npm/Git/local plugin install, manifests, permission policy, restricted macOS execution, trusted-mode confirmation, updates, rollback, and uninstall.
- Pi extension/tool execution and remote-TUI Web adaptation.
- Outline, chapter, review, settings, post-processing, and chat cutover to Pi.
- Release packaging of Node, Pi packages, and runtime assets; the foundation runs from a prepared source checkout and degrades safely otherwise.
- Removal of legacy direct-AI code or the final Pi-only invariant test.
