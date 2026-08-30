import { mkdtemp, readFile, stat, writeFile } from "node:fs/promises";
import { request as httpRequest, type RequestOptions } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { AuthType } from "@earendil-works/pi-ai";
import { afterEach, describe, expect, it } from "vitest";
import { parseRuntimeOptions, validateSocketPath } from "./index.js";
import type { ModelCatalog } from "./models.js";
import type {
  LegacyAPIImport,
  LoginResponse,
  LoginSnapshot,
  SessionSummary,
} from "./protocol.js";
import {
  createRuntimeServer,
  listenOnPrivateSocket,
  type LoginService,
  type RuntimeServices,
  type SessionService,
} from "./server.js";

const fakeKey = "fake-key-never-return";
const fakeToken = "fake-token-never-return";

class FakeModels implements ModelCatalog {
  imported?: LegacyAPIImport;
  loggedOut?: string;
  failProviders = false;

  async listProviders() {
    if (this.failProviders) {
      throw new Error(`provider failed with ${fakeToken}`);
    }
    return [
      {
        id: "fixture",
        name: "Fixture",
        auth_types: ["api_key" as const],
        configured: true,
        models: [
          {
            id: "fixture-model",
            name: "Fixture model",
            provider_id: "fixture",
            reasoning: false,
            context_window: 8_192,
            max_tokens: 1_024,
          },
        ],
      },
    ];
  }

  async listCredentials() {
    return [{ provider_id: "fixture", type: "api_key" as const }];
  }

  async importLegacy(input: LegacyAPIImport) {
    this.imported = input;
    return {
      provider_id: "show-me-the-story-legacy" as const,
      model_id: input.model,
      credential_imported: input.api_key !== undefined,
      available: true,
      made_default: true,
    };
  }

  async logout(providerId: string) {
    this.loggedOut = providerId;
  }
}

class FakeLogins implements LoginService {
  started?: { providerId: string; authType: AuthType };
  status?: { id: string; after: number };
  response?: { id: string; value: LoginResponse };
  cancelled?: string;

  start(providerId: string, authType: AuthType): LoginSnapshot {
    this.started = { providerId, authType };
    return snapshot("login-1");
  }

  snapshot(id: string, afterCursor: number): LoginSnapshot {
    this.status = { id, after: afterCursor };
    return snapshot(id);
  }

  respond(id: string, promptId: string, value: string): void {
    this.response = { id, value: { prompt_id: promptId, value } };
  }

  cancel(id: string): void {
    this.cancelled = id;
  }
}

class FakeSessions implements SessionService {
  selected?: string;

  async select(projectName: string): Promise<SessionSummary> {
    this.selected = projectName;
    return {
      project_id: "project-1",
      session_id: "session-1",
      message_count: 0,
      restored: false,
    };
  }

  current(): SessionSummary | undefined {
    return this.selected === undefined
      ? undefined
      : {
          project_id: "project-1",
          session_id: "session-1",
          message_count: 0,
          restored: false,
        };
  }
}

function snapshot(id: string): LoginSnapshot {
  return {
    id,
    provider_id: "fixture",
    auth_type: "api_key",
    state: "waiting",
    prompt: { id: "prompt-1", type: "secret", message: "API key" },
    events: [],
    next_cursor: 0,
  };
}

interface ResponseView {
  status: number;
  contentType: string | undefined;
  body: string;
  json: unknown;
}

const openServers: Array<ReturnType<typeof createRuntimeServer>> = [];

afterEach(async () => {
  await Promise.all(
    openServers.splice(0).map(
      (server) =>
        new Promise<void>((resolve) => {
          server.close(() => resolve());
        }),
    ),
  );
});

async function serve(services = fakeServices()) {
  const server = createRuntimeServer(services);
  openServers.push(server);
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("expected a TCP test address");
  }
  return {
    services,
    request: (method: string, path: string, body?: string | Buffer) =>
      sendRequest({ hostname: "127.0.0.1", port: address.port, method, path }, body),
  };
}

function fakeServices(): RuntimeServices & {
  models: FakeModels;
  logins: FakeLogins;
  sessions: FakeSessions;
} {
  return {
    models: new FakeModels(),
    logins: new FakeLogins(),
    sessions: new FakeSessions(),
  };
}

function sendRequest(options: RequestOptions, body?: string | Buffer): Promise<ResponseView> {
  return new Promise((resolve, reject) => {
    const request = httpRequest(options, (response) => {
      const chunks: Buffer[] = [];
      response.on("data", (chunk: Buffer) => chunks.push(chunk));
      response.on("end", () => {
        const text = Buffer.concat(chunks).toString("utf8");
        resolve({
          status: response.statusCode ?? 0,
          contentType: response.headers["content-type"],
          body: text,
          json: text === "" ? undefined : JSON.parse(text),
        });
      });
    });
    request.once("error", reject);
    if (body !== undefined) {
      request.setHeader("content-type", "application/json");
      request.end(body);
    } else {
      request.end();
    }
  });
}

function expectJSON(response: ResponseView, status: number, body?: unknown) {
  expect(response.status).toBe(status);
  expect(response.contentType).toMatch(/^application\/json(?:;|$)/);
  if (body !== undefined) {
    expect(response.json).toEqual(body);
  }
  expect(response.body).not.toContain(fakeKey);
  expect(response.body).not.toContain(fakeToken);
}

describe("runtime HTTP routes", () => {
  it("exposes the complete bounded internal API without returning secrets", async () => {
    const fixture = await serve();

    expectJSON(await fixture.request("GET", "/health"), 200, {
      status: "ok",
      protocol_version: 1,
      pi_version: "0.83.0",
    });
    expectJSON(await fixture.request("GET", "/v1/providers"), 200, [
      {
        id: "fixture",
        name: "Fixture",
        auth_types: ["api_key"],
        configured: true,
        models: [
          {
            id: "fixture-model",
            name: "Fixture model",
            provider_id: "fixture",
            reasoning: false,
            context_window: 8_192,
            max_tokens: 1_024,
          },
        ],
      },
    ]);
    expectJSON(await fixture.request("GET", "/v1/credentials"), 200, [
      { provider_id: "fixture", type: "api_key" },
    ]);

    expectJSON(
      await fixture.request(
        "POST",
        "/v1/auth/logins",
        JSON.stringify({ provider_id: "fixture", auth_type: "api_key" }),
      ),
      200,
      snapshot("login-1"),
    );
    expect(fixture.services.logins.started).toEqual({
      providerId: "fixture",
      authType: "api_key",
    });

    expectJSON(await fixture.request("GET", "/v1/auth/logins/login-1?after=12"), 200, snapshot("login-1"));
    expect(fixture.services.logins.status).toEqual({ id: "login-1", after: 12 });

    expectJSON(
      await fixture.request(
        "POST",
        "/v1/auth/logins/login-1/responses",
        JSON.stringify({ prompt_id: "prompt-1", value: fakeToken }),
      ),
      204,
    );
    expect(fixture.services.logins.response).toEqual({
      id: "login-1",
      value: { prompt_id: "prompt-1", value: fakeToken },
    });

    expectJSON(await fixture.request("DELETE", "/v1/auth/logins/login-1"), 204);
    expect(fixture.services.logins.cancelled).toBe("login-1");

    expectJSON(
      await fixture.request(
        "POST",
        "/v1/auth/logout",
        JSON.stringify({ provider_id: "fixture" }),
      ),
      204,
    );
    expect(fixture.services.models.loggedOut).toBe("fixture");

    expectJSON(
      await fixture.request(
        "POST",
        "/v1/legacy-import",
        JSON.stringify({
          base_url: "http://127.0.0.1:11434/v1",
          model: "fixture-model",
          api_key: fakeKey,
          max_tokens: 4096,
        }),
      ),
      200,
      {
        provider_id: "show-me-the-story-legacy",
        model_id: "fixture-model",
        credential_imported: true,
        available: true,
        made_default: true,
      },
    );
    expect(fixture.services.models.imported?.api_key).toBe(fakeKey);

    expectJSON(
      await fixture.request(
        "POST",
        "/v1/sessions/select",
        JSON.stringify({ project_name: "Fixture Story" }),
      ),
      200,
      {
        project_id: "project-1",
        session_id: "session-1",
        message_count: 0,
        restored: false,
      },
    );
    expect(fixture.services.sessions.selected).toBe("Fixture Story");
    expectJSON(await fixture.request("GET", "/v1/sessions/current"), 200, {
      project_id: "project-1",
      session_id: "session-1",
      message_count: 0,
      restored: false,
    });
  });

  it("returns JSON errors for routing, validation, size, and service failures", async () => {
    const fixture = await serve();

    expectJSON(await fixture.request("POST", "/health", "{}"), 405, {
      error: { code: "method_not_allowed", message: "method not allowed" },
    });
    expectJSON(await fixture.request("GET", "/missing"), 404, {
      error: { code: "not_found", message: "route not found" },
    });
    expectJSON(await fixture.request("POST", "/v1/auth/logout", "{"), 400, {
      error: { code: "invalid_request", message: "request body must be valid JSON" },
    });
    expectJSON(
      await fixture.request("POST", "/v1/auth/logout", JSON.stringify({ provider_id: " " })),
      400,
      { error: { code: "invalid_request", message: "provider_id is required" } },
    );
    expectJSON(await fixture.request("GET", "/v1/auth/logins/login-1?after=1.5"), 400, {
      error: { code: "invalid_request", message: "after must be a non-negative integer" },
    });
    expectJSON(
      await fixture.request("POST", "/v1/auth/logout", Buffer.alloc(1_048_577, 97)),
      413,
      { error: { code: "payload_too_large", message: "request body exceeds 1 MiB" } },
    );

    fixture.services.models.failProviders = true;
    expectJSON(await fixture.request("GET", "/v1/providers"), 500, {
      error: { code: "internal_error", message: "internal runtime error" },
    });
  });

  it("returns null when no story session is selected", async () => {
    const fixture = await serve();
    expectJSON(await fixture.request("GET", "/v1/sessions/current"), 200, null);
  });
});

describe("private Unix socket lifecycle", () => {
  it("serves with mode 0600 and removes the socket on close", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-server-"));
    const socketPath = join(root, "pi.sock");
    const server = createRuntimeServer(fakeServices());
    const close = await listenOnPrivateSocket(server, socketPath);

    expect((await stat(socketPath)).mode & 0o777).toBe(0o600);
    expectJSON(await sendRequest({ socketPath, method: "GET", path: "/health" }), 200, {
      status: "ok",
      protocol_version: 1,
      pi_version: "0.83.0",
    });

    await close();
    await expect(stat(socketPath)).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("refuses to replace a regular file at the socket path", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-server-file-"));
    const socketPath = join(root, "pi.sock");
    await writeFile(socketPath, "keep-me");

    await expect(
      listenOnPrivateSocket(createRuntimeServer(fakeServices()), socketPath),
    ).rejects.toThrow("non-socket");
    await expect(readFile(socketPath, "utf8")).resolves.toBe("keep-me");
  });
});

describe("runtime startup validation", () => {
  it("requires absolute data and socket paths and confines the socket to runDir", () => {
    const root = join(tmpdir(), "story-pi-options");
    const runDir = join(root, "run");
    const socket = join(runDir, "pi.sock");

    expect(parseRuntimeOptions(["--data-dir", root, "--socket", socket])).toEqual({
      dataDir: root,
      socketPath: socket,
    });
    expect(() => parseRuntimeOptions(["--data-dir", "relative", "--socket", socket])).toThrow(
      "absolute",
    );
    expect(() => parseRuntimeOptions(["--data-dir", root])).toThrow("--socket");
    expect(() => validateSocketPath(join(root, "outside.sock"), runDir)).toThrow("outside");
    expect(validateSocketPath(socket, runDir)).toBe(socket);
  });
});
