# Pi Web Host Redesign

**Status:** Approved design

**Date:** 2026-08-30

**Target:** Local, single-user macOS installation of `show-me-the-story`

## Summary

Replace the application's fixed Go-managed LLM pipeline with an embedded Pi runtime while preserving the Go backend as the canonical novel-domain core and the Svelte application as the editing and review surface.

The new Node runtime embeds the official `@earendil-works/pi-coding-agent` SDK. It owns model calls, persistent agent sessions, extension lifecycle, skills, prompts, themes, provider plugins, and task orchestration. The Go process remains the only component allowed to commit canonical story data. A Web extension UI bridge adapts standard Pi interactions to native Svelte components and renders arbitrary Pi TUI components in a browser compatibility surface.

The application uses its own Pi configuration and data directories. It does not read or modify the user's normal `~/.pi/agent` installation.

## Motivation

The current system has two AI paths:

1. Go code directly calls an OpenAI-compatible chat-completions API and runs a fixed sequence for outline generation, chapter drafting, summarization, fact checking, memory extraction, revision, and post-processing.
2. A custom assistant loop in `agent.go` parses model-produced tool calls and drives the same domain actions.

An optional MCP sidecar exposes a useful author-mode boundary, but it does not turn the application into an extensible agent host. The fixed pipeline limits how models and third-party tools can plan, critique, revise, preserve session state, or add new interaction surfaces.

This redesign makes Pi the only AI runtime so installed Pi packages can influence the real outline, writing, revision, and post-processing workflows rather than only an assistant chat.

## Goals

1. Embed Pi as the sole runtime for every AI-dependent action.
2. Support Pi extensions, skills, prompts, themes, packages, custom providers, session state, commands, shortcuts, and public Extension UI APIs.
3. Adapt Pi UI extensions to the Web application, including complex `ctx.ui.custom()` TUI components.
4. Provide Web plugin management for npm, Git, and local-path packages: install, enable, disable, update, uninstall, permission review, health checks, and rollback.
5. Provide restricted and fully trusted plugin modes.
6. Support Pi's complete provider and model system: API keys, OAuth/subscription login, custom providers registered by extensions, and import from the existing `api.json`.
7. Keep current story projects readable and editable without manual migration.
8. Preserve Go's task locking, domain state transitions, deterministic validation, and persistence as the only canonical write path.
9. Keep chapter and outline review human-controlled by default.
10. Target the user's current local macOS environment for the first release.

## Non-goals

1. Multi-user, LAN, or Internet deployment.
2. Restricted-plugin sandboxing on Windows or Linux in the first release.
3. Reusing or modifying `~/.pi/agent`.
4. Allowing plugins to edit canonical files under `storys/` directly.
5. Automatically translating arbitrary TUI source code into idiomatic Svelte source code. Generic TUI compatibility uses a remote renderer; selected plugins may later receive dedicated Svelte skins.
6. Maintaining a private fork of Pi.
7. Treating Pi conversation history as the source of truth for story facts.
8. Destructive bulk conversion of existing story data.
9. Preserving Go's direct model-calling pipeline after final cutover.

## Approved Architecture

```text
Browser / Svelte
  - Variable dual workspace
  - Story artifact editor and review surface
  - Pi transcript, tasks, tools, and session controls
  - Native Extension UI host
  - TUI compatibility surface
  - Plugin and model management
             |
             | same-origin HTTP + WebSocket
             v
Go application
  - Serves the Web app
  - Proxies the private Pi event channel
  - Canonical REST/domain APIs
  - Task locking and state machine
  - Deterministic validation
  - Story persistence
             |
             | private loopback/Unix-socket API
             v
Node Pi runtime
  - AgentSession registry
  - ModelRuntime and authentication
  - ResourceLoader and package registry
  - Story Tool adapter
  - Web ExtensionUIContext
  - TUI renderer
  - Plugin sandbox supervisor
             |
             +---- restricted plugin helper processes
             +---- fully trusted Pi extensions
```

### Process topology

- The Go application supervises one long-lived Node Pi runtime process.
- Production browser traffic reaches Pi through same-origin Go routes. The Pi runtime does not expose an unauthenticated public port.
- The Go-to-Node channel uses a Unix-domain socket under the application Pi data directory. Development may use a token-authenticated loopback TCP port.
- Only the selected story's Pi session is active. Other story sessions are persisted and lazily restored when the user switches projects.
- Restricted plugins run in helper processes. Fully trusted extensions use Pi's standard in-process loader for maximum compatibility.
- If a fully trusted extension terminates the runtime process, Go restarts the runtime, restores the selected story session, and quarantines the last extension being loaded until the user re-enables it.

### Runtime dependency

- The runtime uses the current official package scope, `@earendil-works/pi-coding-agent`, and pins the selected Pi version in `package-lock.json`.
- Pi 0.83 requires Node.js 22.19 or newer. The existing MCP sidecar's Node 18 declaration is not sufficient for the new runtime.
- The implementation must not bundle Pi into a single JavaScript file unless Pi's package-adjacent runtime assets and metadata are preserved. The initial implementation runs compiled application files with normal `node_modules` resolution.

## Component Responsibilities

### Svelte Web application

The selected layout is **Variable Dual Workspace**:

- The left navigation continues to expose project, settings, characters, outline, writing, memory, foreshadowing, and post-processing views.
- The center artifact pane displays the selected structured artifact or manuscript.
- The right Pi pane displays the persistent conversation, streaming output, task state, tool calls, plugin status, and commands.
- The divider is resizable. Either pane may expand to full width.
- Plugin surfaces may dock in either pane, open as an overlay, or enter full-screen mode.
- The review view displays prose or outline diffs alongside deterministic warnings, plugin findings, unresolved issues, context version, model/provider identity, plugin versions, and task/session identifiers.

The browser never receives unmasked secrets. OAuth URLs, device codes, and masked credential metadata are allowed; tokens and API keys remain in the Pi runtime's credential store.

### Node Pi runtime

The runtime owns:

- `AgentSession` creation, persistence, recovery, branching, compaction, steering, follow-up messages, and cancellation.
- `ModelRuntime`, provider discovery, OAuth/login flow, model selection, thinking level, and provider plugins.
- `DefaultResourceLoader` discovery for the application-owned global and story-local resources.
- Pi package discovery and activation.
- Story Tool registration and dispatch.
- Pi event streaming to the browser.
- Web implementations of `ExtensionUIContext`.
- Restricted plugin proxy processes and permission enforcement.
- Plugin diagnostics, activation transactions, and version rollback.

It does not write canonical story files.

### Go domain core

Go remains responsible for:

- Project selection and canonical storage paths.
- Settings, characters, organizations, relations, worldview, outline, chapters, memory, and foreshadow data.
- Task ownership, cancellation state, mutual exclusion, and current-project transitions.
- Allowed domain transitions such as pending → review → confirmed.
- Deterministic validation such as required fields, prose length, valid chapter frontier, context version, referenced IDs, and duplicate updates.
- Atomic persistence and backups.
- User confirmation for destructive or finalizing operations.
- Serving and proxying the browser application.

Go's LLM request functions and prompt orchestration remain temporarily available only during staged migration. They are removed after the Pi-only acceptance gate passes.

### Existing MCP sidecar

The optional `mcp/` integration remains isolated during the migration so existing Codex workflows are not broken accidentally. It continues to call Go domain APIs and never becomes the Pi runtime transport. After Pi cutover, its tools may continue to manipulate story data, but they do not provide an alternative model runtime.

## Storage Layout

The exact application data root continues to follow the application's existing program/data directory rules. Under that root, Pi uses:

```text
pi-data/
  agent/
    settings.json
    auth.json
    models.json
    trust.json
    npm/
    git/
    extensions/
    skills/
    prompts/
    themes/
  projects/
    <stable-project-id>/
      workspace/
        .pi/
      sessions/
      plugin-data/
      scratch/
  staging/
  rollback/
  run/
    pi.sock
```

Rules:

- The runtime always passes an explicit application-owned `agentDir`; it does not fall back to `~/.pi/agent`.
- A story's Pi `cwd` is its isolated workspace, not its canonical `storys/<name>` directory.
- Project-local `.pi` resources reside in that workspace.
- Canonical story content is read and changed through Story Tools.
- Restricted plugins see only approved workspace and plugin-data paths.
- Credential files use owner-only filesystem permissions.

## Story Tool Contract

Existing REST endpoints are reused where their semantics are already safe. New agent-facing endpoints normalize generation into a versioned draft transaction.

### Task lifecycle

1. `begin_task(kind, target, request)`
   - Validates current project and state.
   - Acquires task ownership.
   - Returns a stable `task_id`, `context_version`, binding constraints, and a task-specific context snapshot.
2. Pi plans and generates using the snapshot, installed resources, and model.
3. `submit_draft(task_id, context_version, artifact)`
   - Rejects stale context versions.
   - Deterministically validates the typed artifact.
   - Atomically stores the pending draft and structured side effects.
   - Moves the artifact into review, never directly to confirmed unless explicit auto-confirm is enabled.
4. The user edits, returns feedback to Pi, or confirms from the Web review surface.
5. Confirmation atomically applies the draft and advances canonical state.

Cancellation aborts active model/tool work and releases task ownership without altering the last confirmed artifact.

### Typed artifacts

Artifact payloads are a discriminated union, not an unvalidated generic JSON blob.

- Outline draft: title, core prompt, synopsis, chapter outline entries, assumptions, unresolved findings, and provenance.
- Chapter draft: title, content, summary, extracted facts, character deltas, timeline deltas, foreshadow updates, memory entries, unresolved findings, and provenance.
- Revision draft: target artifact/version, replacement content or structured patch, downstream impact notes, updated side effects, unresolved findings, and provenance.
- Post-processing draft: target range, diagnoses, ordered changes, per-change evidence, resulting content or patches, and provenance.

Provenance records model/provider identity, Pi version, active plugin versions, session/task identifiers, context version/hash, and tool events. It does not store private chain-of-thought.

### Canonical-state rule

Pi conversation history stores discussion, preferences, tool traces, and extension state. It may be compacted or branched. It is never trusted as the sole source for characters, chronology, confirmed prose, foreshadows, or story memory.

Every generation task receives a fresh versioned snapshot from Go. A draft produced against an outdated snapshot is rejected and must be rebased or regenerated.

## AI Workflow

All AI-dependent buttons and chat commands create typed Pi tasks. They no longer invoke Go's `CallAPI*` functions directly.

The common workflow is:

1. Receive a user request or typed UI action.
2. Begin a task and freeze a versioned domain snapshot.
3. Let Pi plan and generate with extension hooks, skills, prompts, and tools.
4. Run installed quality plugins and deterministic validators.
5. Let Pi revise or explicitly report remaining issues.
6. Submit one atomic artifact bundle to review.
7. Display prose/diffs and evidence in the Web review surface.
8. Confirm, edit, or return annotated feedback to the same story session.

Manual confirmation is the default. The current auto-confirm behavior remains an explicit project-level option. Without it, neither Pi nor a plugin may call the final confirmation capability.

## Pi Extension Compatibility

The compatibility target is Pi's documented public Extension API for the pinned runtime version.

| Pi capability | Web behavior |
| --- | --- |
| Tools and lifecycle events | Registered with the active `AgentSession`; calls and updates appear in the transcript. |
| Commands | Added to the Web command palette and accepted as slash commands. |
| Skills and prompt templates | Discovered by the application-owned `ResourceLoader`; enabled/disabled in plugin management. |
| Providers | Registered with Pi's model runtime and shown in model management. |
| `select`, `confirm`, `input`, `editor` | Native Svelte dialogs and editors with request/response correlation and timeouts. |
| `notify` | Web notification center and toast. |
| `setStatus`, working messages, widgets | Pi pane status bar and areas above/below its composer. |
| Shortcuts | Browser shortcut registry, scoped to the Pi/plugin surface; conflicts require user choice. |
| Flags | Plugin settings controls and persisted plugin configuration. |
| Themes | Semantic Pi colors mapped to the plugin surface; TUI renderer receives the original Pi theme. |
| `ctx.ui.custom()` | Remote TUI component surface, docked/overlay/full-screen according to options. |
| Custom header/footer/editor | Pi pane chrome or a focused remote TUI surface. |
| Custom message/tool renderers | Rendered as remote TUI blocks inside the transcript. |
| Raw terminal input | Routed only while the relevant plugin surface owns keyboard focus. |

Extensions that depend on private Pi internals rather than the public API are not guaranteed to work. The plugin manager reports the incompatibility and may offer fully trusted mode, but it does not silently weaken security.

## TUI Compatibility Renderer

Pi TUI components expose terminal-width rendering and raw input handling. The generic Web adapter keeps the component in the Node process and transports frames rather than attempting to transpile component source.

### Frame protocol

Each surface has a stable `surface_id`, plugin identity, session identity, monotonically increasing sequence, width, height, overlay metadata, styled text lines, and focus state.

- The runtime calls `render(width)` when the component invalidates, its container resizes, or theme data changes.
- ANSI/style output is parsed and sanitized before the browser applies it.
- The browser sends normalized key/input events only to the focused surface.
- `done(result)` closes the surface and resolves the original extension call.
- Out-of-order frames are dropped by sequence number.
- Closing a browser tab cancels or detaches pending interactive UI according to the extension call's timeout; it must not leave the session permanently blocked.

Standard dialog methods use native Svelte UI instead of the frame protocol.

## Plugin Management

### Supported sources

- npm package specifiers, including pinned versions.
- Git URLs/specifiers, including pinned tags or commits.
- Local files or directories in developer-link mode.

The manager follows Pi's package manifest and conventional resource directories for extensions, skills, prompts, and themes.

### Installation transaction

1. Resolve the exact source and version/ref.
2. Fetch into `pi-data/staging` without executing dependency lifecycle scripts.
3. Inspect package metadata, Pi manifests, dependency graph, hashes, changed source, native modules, lifecycle scripts, and high-risk code patterns.
4. Present source identity, requested/effective permissions, detected risks, and version changes.
5. Require the user to choose restricted or fully trusted mode. High-risk capabilities require a second confirmation.
6. Install dependencies using production dependency semantics. Restricted installs keep scripts disabled. A package that requires install scripts must be fully trusted and explicitly approved.
7. Perform an isolated load and registration smoke test.
8. Atomically activate the new version and reload resources after active tool calls drain.
9. Preserve the previous working version for rollback.

Uninstall first disables and unloads the package. Files are removed only after no active session or tool owns them. Plugin data is retained by default and deleted only through a separate explicit action.

### Local developer links

Local-path packages remain linked rather than copied. The manager records the resolved absolute path and current hash, displays a developer-mode badge, detects changes, and offers manual reload. Restricted mode exposes the link read-only unless write access was separately approved.

## Plugin Trust Modes

### Restricted mode

Restricted mode is the default.

- The plugin runs in a helper process with a mirrored public `ExtensionAPI`.
- Registrations, events, tool executions, UI calls, state operations, and provider calls cross a typed local RPC boundary.
- Executable callbacks remain in the helper. Tools/providers in the main session are RPC proxies.
- TUI components render inside the helper and send sanitized frames through the runtime.
- Environment variables are scrubbed except for explicitly approved names.
- Filesystem, child process, and native module access use Node's permission boundary.
- macOS process sandbox policy restricts filesystem and network access.
- Canonical story storage is never mounted as writable; approved story operations use scoped Story Tools.
- Crashes disable only the plugin helper.

The current target Mac exposes `/usr/bin/sandbox-exec`; the implementation may use it as the macOS policy launcher together with Node permissions. Because platform sandbox facilities can change, runtime startup performs an enforcement self-test that attempts denied file, network, and process operations. Restricted mode is available only if every denial succeeds. If self-test fails, the UI offers cancel or explicit fully trusted mode; it never labels the plugin restricted.

### Fully trusted mode

- The plugin is loaded through Pi's standard resource loader with the current user's permissions.
- It can use arbitrary Node APIs, native dependencies, installation scripts, files, network access, and child processes.
- The UI continuously marks it as fully trusted.
- Activation still records exact source/version/hash, runs health checks, and supports rollback.
- JavaScript errors are contained by Pi's extension error handling. A process-level crash triggers runtime restart and quarantine of the last-loading extension.

Static analysis is advisory. It cannot prove third-party JavaScript safe. Runtime isolation is the enforcement mechanism for restricted mode.

## Provider and Credential Management

- Pi's application-owned `ModelRuntime` is the source of provider/model state.
- The Web model page lists built-in and extension-registered providers and models.
- API-key login stores secrets in application-owned `auth.json` with owner-only permissions.
- OAuth and subscription login use Pi's provider flow. The runtime sends authorization URLs, device codes, and user prompts to the Web UI and receives correlated responses.
- Provider plugins loaded from approved packages appear after resource reload.
- The browser displays masked metadata only.

### Legacy `api.json` import

The importer reads the existing base URL, model, API key, and strict-URL behavior, previews the mapping, and creates an equivalent Pi OpenAI-compatible/custom provider entry.

- Import copies credentials; it does not delete or rewrite `api.json`.
- The imported provider must pass a model availability/authentication check before it becomes the default.
- The user may remove the legacy file later through a separate explicit action after Pi-only verification.

## Error Handling and Recovery

- **Provider/authentication failure:** Keep the task checkpoint and draft inputs. Let the user repair login or select another model, then retry.
- **Plugin JavaScript error:** Report the plugin and event/tool, continue the session where Pi allows, and offer disable/retry.
- **Restricted helper crash:** Disable the helper, preserve the session, and mark any active tool call failed.
- **Trusted runtime crash:** Go restarts Node, restores the session, and quarantines the last-loading plugin.
- **Validation failure:** Return structured errors to Pi for correction. Do not overwrite a confirmed artifact.
- **Context version conflict:** Reject submission and request a fresh snapshot/rebase.
- **Browser refresh/disconnect:** Replay session/task events from a cursor. Interactive requests remain pending only until their defined timeout.
- **Cancellation:** Abort Pi/model/tool work and preserve the last confirmed state.
- **Plugin update failure:** Keep the previous active version and surface staging/load diagnostics.
- **Go or Node restart:** Reconcile task ownership from persisted checkpoints before accepting new writes.

## Migration Strategy

This document is the approved umbrella design. Its implementation is intentionally split into five separately reviewed plans; no single branch or plan attempts the entire redesign.

1. **Foundation plan:** Phase 0 characterization plus Phase 1 Pi runtime, supervision, independent storage, provider/auth, and legacy API import. It introduces no production story writes.
2. **Domain and workspace plan:** Versioned Story Tools, task/draft contracts, event proxy, persistent per-story sessions, and the variable dual workspace. It migrates chat only.
3. **Plugin platform plan:** Package management, native Extension UI, TUI compatibility rendering, trust modes, macOS enforcement self-test, hot reload, and rollback.
4. **AI workflow migration plans:** One independently releasable plan for each slice listed in Phase 4. Each slice defines its own rollback flag and proves that only one runtime has write authority.
5. **Pi-only cutover plan:** Removes remaining Go model paths only after every prior acceptance gate passes.

Each plan starts only after the preceding plan's tests and user review pass. Shared contracts from this design may be extended additively, but responsibility boundaries and canonical-state rules require a design amendment.

### Phase 0: Characterize the current system

- Add regression fixtures for opening existing projects, project switching, config/state reading, outline and chapter review, import, confirmation, and cancellation.
- Capture current story schemas and representative existing projects as anonymized test fixtures.
- Add assertions around every place that can write canonical story data.

### Phase 1: Add the Pi runtime without production writes

- Create the Node runtime package with Node 22.19+ requirement and pinned Pi SDK.
- Add application-owned storage, model runtime, provider login, legacy API import, and persistent sessions.
- Start/stop it under Go supervision.
- Do not route existing generation buttons through it yet.

### Phase 2: Add the domain bridge and dual workspace

- Add the versioned task/draft contract and Story Tools.
- Add same-origin WebSocket event proxy and replay.
- Implement the approved variable dual workspace.
- Move the assistant/chat experience to Pi first.

### Phase 3: Add the plugin platform

- Implement npm, Git, and local-path management.
- Implement native standard Extension UI mapping.
- Implement the TUI frame/input bridge.
- Implement restricted helpers, trusted activation, permission UI, update, reload, quarantine, and rollback.
- Pass the plugin compatibility and sandbox fixture suites before AI workflow cutover.

### Phase 4: Replace AI workflows by slice

Migrate in this order:

1. Outline generation and revision.
2. Chapter generation.
3. Chapter-specific revision and polishing.
4. Transition smoothing and memory/foreshadow updates.
5. Full-book diagnosis, roadmap, and execution.

During a slice's migration, only one runtime has write authority for that operation. A local migration flag may retain the legacy path as a rollback mechanism, but the two paths never write concurrently.

### Phase 5: Pi-only cutover

- Verify every AI request is emitted by Pi's model runtime.
- Remove Go direct model calls, retry loops, tool-call parsing, and prompts that became unreachable.
- Preserve deterministic context builders, validators, domain tools, and data migrations.
- Keep non-destructive backups and legacy import records.

## Data Migration and Backups

- Existing `storys` projects continue to open without a bulk rewrite.
- New review drafts, provenance, and context versions use additive files or backward-compatible optional fields.
- Before first Pi enablement and before any schema upgrade, create a timestamped, restorable project snapshot.
- Backup creation is verified before migration writes begin.
- `api.json` is copied, never moved or deleted automatically.
- Plugin package activation is versioned and atomic; plugin data is separate from package files.

## Testing Strategy

### Go tests

- Task ownership and mutual exclusion.
- Context snapshot versions and stale-submit rejection.
- Typed artifact validation.
- Atomic review drafts and confirmation.
- Cancellation and restart reconciliation.
- Existing project/schema regression.

### Pi runtime tests

- Fake-model tests for streaming, tool loops, cancellation, compaction, restore, branching, model switching, and provider errors.
- Resource discovery and reload with the explicit application `agentDir` and story workspace.
- Credential masking and OAuth request/response correlation.
- Verification that browser and logs never expose secrets.

### Plugin compatibility fixtures

Fixtures cover every documented public capability used by the design:

- Tools, tool updates, events, commands, shortcuts, flags, skills, prompts, themes, provider registration, package dependencies, and persisted extension state.
- Native dialogs, notifications, status, widgets, title, working indicators, custom header/footer/editor, message/tool renderers, raw input, overlays, and `ctx.ui.custom()` completion values.
- Reload, enable/disable, update, uninstall, and rollback while sessions exist.

### Security fixtures

Malicious restricted plugins attempt:

- Reading or writing denied files, including canonical story data and credential files.
- Connecting to denied network targets.
- Starting denied child processes.
- Reading denied environment variables.
- Escaping local-path and symlink containment.
- Sending unsafe ANSI/HTML content through the TUI renderer.

The startup sandbox self-test and the fixture suite must prove denial. A failed assertion disables restricted mode.

### Browser end-to-end tests

- Install, configure, update, disable, rollback, and uninstall npm, Git, and local packages.
- Complete native Extension UI and complex TUI keyboard interactions.
- Resolve shortcut conflicts and resize/dock/full-screen plugin surfaces.
- Refresh/reconnect while generation and interactive extension UI are active.
- Run the complete existing-project journey: open → import/select provider → generate/revise outline → draft chapter → return feedback → confirm → restart → resume.

### Pi-only verification

The final integration suite observes outbound model traffic and fails if any AI request originates from Go's legacy `CallAPI*` path.

## Acceptance Criteria

1. An existing story opens and continues without manual schema conversion.
2. Each story has an isolated, persistent Pi session using application-owned configuration.
3. npm, Git, and local-path Pi packages can be installed, enabled, disabled, updated, rolled back, and uninstalled in the Web UI.
4. Standard Extension UI is native Web UI; complex public Pi TUI components remain interactive in the compatibility surface.
5. Restricted plugins cannot perform denied file, network, process, or environment operations on the target Mac.
6. If sandbox enforcement fails, restricted mode is unavailable rather than silently weakened.
7. Fully trusted mode is clearly labeled and requires explicit second confirmation for dangerous capabilities or scripts.
8. Pi's built-in providers, API keys, OAuth/subscription logins, extension providers, and legacy `api.json` import work from the Web model manager.
9. Outline, chapter, revision, polish, transition, memory/foreshadow, and full-book AI operations are all orchestrated by Pi.
10. Every submitted artifact carries a context version and provenance and enters review before confirmation unless auto-confirm was explicitly enabled.
11. Plugins cannot directly commit canonical story data.
12. Model, plugin, browser, or runtime failures do not corrupt the last confirmed content.
13. Plugin activation/update failure automatically retains or restores the last working version.
14. Final verification proves that Go no longer emits model requests.

## Source References

- [Pi SDK](https://pi.dev/docs/latest/sdk)
- [Pi extensions](https://pi.dev/docs/latest/extensions)
- [Pi RPC limitations](https://pi.dev/docs/latest/rpc)
- [Pi documentation and installation](https://pi.dev/docs/latest)
- [Pi containerization and sandbox options](https://pi.dev/docs/latest/containerization)
- [Pi package scope migration](https://pi.dev/news/2026/5/7/pi-has-a-new-home)
