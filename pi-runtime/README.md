# Show Me The Story Pi Runtime

This directory contains the private Node/TypeScript runtime used by the Pi migration foundation. It embeds `@earendil-works/pi-coding-agent` and `@earendil-works/pi-ai` 0.83.0 and currently supports local macOS use with Node.js >= 22.19.0.

The runtime is not a public network service. Go starts it with absolute paths:

```bash
node dist/index.js --data-dir /absolute/program/pi-data --socket /absolute/program/pi-data/run/pi.sock
```

Both arguments are required. The socket must be inside `<data-dir>/run`, is created with mode `0600`, and is removed during a clean shutdown. A regular file at the requested socket path is never replaced.

## Private protocol

Protocol version 1 is JSON over HTTP on the Unix socket. The only routes are:

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

This API is an implementation boundary between the local Go process and its owned Node child. It is not stable for third-party callers. Request bodies are capped at 1 MiB; errors are structured JSON; unknown failures are redacted.

## Data ownership and security

- All paths are rooted under the explicit `--data-dir`; the runtime never reads `~/.pi`.
- `agent/auth.json`, `agent/models.json`, and `agent/models-store.json` are application-owned Pi files. Secret-bearing JSON is written atomically with mode `0600`; private directories use `0700`.
- Each story uses `projects/<normalized-name-hash>/` with its own workspace, `.pi`, sessions, plugin-data, and scratch directories. The Node runtime never receives or inspects the Go-owned `storys/**` path.
- Go remains the only canonical writer for story configuration, progress, settings, outlines, and chapters.
- API keys are request-only values. Provider and credential responses expose metadata, never secret material. Existing `api.json` import is merge/copy-only.
- Foundation sessions start with all tools, extensions, skills, prompt templates, themes, and project context files disabled. Plugin installation, trust decisions, sandbox helpers, and Web UI surfaces belong to the later plugin phase.
- Child stdout/stderr is drained into a bounded 32-KiB buffer and discarded; it is never forwarded to the browser or application logger.

## Development

```bash
npm ci
npm test
npm run typecheck
npm run build
```

`npm test` runs source tests only. `npm run build` uses `tsconfig.build.json` and emits production files without compiling tests into a clean `dist/` directory. From the repository root, `task pi:test` and `task pi:build` provide the same checks.
