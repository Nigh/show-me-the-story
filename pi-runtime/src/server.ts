import { chmod, lstat, unlink } from "node:fs/promises";
import {
  createServer,
  type IncomingMessage,
  type Server,
  type ServerResponse,
} from "node:http";
import {
  LoginConflictError,
  LoginNotFoundError,
  LoginValidationError,
} from "./login-broker.js";
import type { ModelCatalog } from "./models.js";
import type {
  AuthType,
  HealthSummary,
  LegacyAPIImport,
  LoginSnapshot,
  SessionSummary,
} from "./protocol.js";

const maximumBodyBytes = 1_048_576;

export interface LoginService {
  start(providerId: string, authType: AuthType): LoginSnapshot;
  snapshot(id: string, afterCursor: number): LoginSnapshot;
  respond(id: string, promptId: string, value: string): void;
  cancel(id: string): void;
}

export interface SessionService {
  select(projectName: string): Promise<SessionSummary>;
  current(): SessionSummary | undefined;
}

export interface RuntimeServices {
  models: ModelCatalog;
  logins: LoginService;
  sessions: SessionService;
}

export function createRuntimeServer(services: RuntimeServices): Server {
  return createServer((request, response) => {
    void routeRequest(services, request, response).catch((error: unknown) => {
      if (response.headersSent) {
        response.destroy();
        return;
      }
      if (error instanceof HTTPError) {
        writeError(response, error.status, error.code, error.message);
        return;
      }
      if (error instanceof LoginNotFoundError) {
        writeError(response, 404, error.code, error.message);
        return;
      }
      if (error instanceof LoginValidationError) {
        writeError(response, 400, error.code, error.message);
        return;
      }
      if (error instanceof LoginConflictError) {
        writeError(response, 409, error.code, error.message);
        return;
      }
      writeError(response, 500, "internal_error", "internal runtime error");
    });
  });
}

export async function listenOnPrivateSocket(
  server: Server,
  socketPath: string,
): Promise<() => Promise<void>> {
  await removeSocketIfPresent(socketPath, true);
  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error) => reject(error);
    server.once("error", onError);
    server.listen(socketPath, () => {
      server.off("error", onError);
      resolve();
    });
  });

  try {
    await chmod(socketPath, 0o600);
  } catch (error) {
    await closeServer(server);
    await removeSocketIfPresent(socketPath, false);
    throw error;
  }

  let closed = false;
  return async () => {
    if (closed) {
      return;
    }
    closed = true;
    await closeServer(server);
    await removeSocketIfPresent(socketPath, false);
  };
}

async function routeRequest(
  services: RuntimeServices,
  request: IncomingMessage,
  response: ServerResponse,
): Promise<void> {
  const method = request.method ?? "";
  const url = new URL(request.url ?? "/", "http://pi-runtime");
  const allowed = allowedMethods(url.pathname);
  if (allowed === undefined) {
    throw new HTTPError(404, "not_found", "route not found");
  }
  if (!allowed.includes(method)) {
    throw new HTTPError(405, "method_not_allowed", "method not allowed");
  }

  if (method === "GET" && url.pathname === "/health") {
    const health: HealthSummary = {
      status: "ok",
      protocol_version: 1,
      pi_version: "0.83.0",
    };
    writeJSON(response, 200, health);
    return;
  }
  if (method === "GET" && url.pathname === "/v1/providers") {
    writeJSON(response, 200, await services.models.listProviders());
    return;
  }
  if (method === "GET" && url.pathname === "/v1/credentials") {
    writeJSON(response, 200, await services.models.listCredentials());
    return;
  }
  if (method === "POST" && url.pathname === "/v1/auth/logins") {
    const body = await readJSONObject(request);
    const providerId = requiredString(body, "provider_id");
    const authType = requiredString(body, "auth_type");
    if (authType !== "api_key" && authType !== "oauth") {
      throw new HTTPError(400, "invalid_request", "auth_type must be api_key or oauth");
    }
    writeJSON(response, 200, services.logins.start(providerId, authType));
    return;
  }

  const responseMatch = url.pathname.match(/^\/v1\/auth\/logins\/([^/]+)\/responses$/);
  if (method === "POST" && responseMatch !== null) {
    const body = await readJSONObject(request);
    services.logins.respond(
      decodePathPart(responseMatch[1]!),
      requiredString(body, "prompt_id"),
      requiredString(body, "value"),
    );
    writeJSON(response, 204);
    return;
  }

  const loginMatch = url.pathname.match(/^\/v1\/auth\/logins\/([^/]+)$/);
  if (loginMatch !== null) {
    const id = decodePathPart(loginMatch[1]!);
    if (method === "GET") {
      writeJSON(response, 200, services.logins.snapshot(id, parseCursor(url.searchParams.get("after"))));
      return;
    }
    services.logins.cancel(id);
    writeJSON(response, 204);
    return;
  }

  if (method === "POST" && url.pathname === "/v1/auth/logout") {
    const body = await readJSONObject(request);
    await services.models.logout(requiredString(body, "provider_id"));
    writeJSON(response, 204);
    return;
  }
  if (method === "POST" && url.pathname === "/v1/legacy-import") {
    const body = await readJSONObject(request);
    const input: LegacyAPIImport = {
      base_url: requiredString(body, "base_url"),
      model: requiredString(body, "model"),
    };
    const apiKey = optionalString(body, "api_key");
    if (apiKey !== undefined) {
      input.api_key = apiKey;
    }
    const maxTokens = optionalPositiveInteger(body, "max_tokens");
    if (maxTokens !== undefined) {
      input.max_tokens = maxTokens;
    }
    writeJSON(response, 200, await services.models.importLegacy(input));
    return;
  }
  if (method === "POST" && url.pathname === "/v1/sessions/select") {
    const body = await readJSONObject(request);
    writeJSON(response, 200, await services.sessions.select(requiredString(body, "project_name")));
    return;
  }
  if (method === "GET" && url.pathname === "/v1/sessions/current") {
    writeJSON(response, 200, services.sessions.current() ?? null);
    return;
  }

  throw new HTTPError(404, "not_found", "route not found");
}

function allowedMethods(pathname: string): string[] | undefined {
  const fixed = new Map<string, string[]>([
    ["/health", ["GET"]],
    ["/v1/providers", ["GET"]],
    ["/v1/credentials", ["GET"]],
    ["/v1/auth/logins", ["POST"]],
    ["/v1/auth/logout", ["POST"]],
    ["/v1/legacy-import", ["POST"]],
    ["/v1/sessions/select", ["POST"]],
    ["/v1/sessions/current", ["GET"]],
  ]);
  const staticMethods = fixed.get(pathname);
  if (staticMethods !== undefined) {
    return staticMethods;
  }
  if (/^\/v1\/auth\/logins\/[^/]+\/responses$/.test(pathname)) {
    return ["POST"];
  }
  if (/^\/v1\/auth\/logins\/[^/]+$/.test(pathname)) {
    return ["GET", "DELETE"];
  }
  return undefined;
}

async function readJSONObject(request: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const rawChunk of request) {
    const chunk = Buffer.isBuffer(rawChunk) ? rawChunk : Buffer.from(rawChunk);
    size += chunk.length;
    if (size > maximumBodyBytes) {
      request.resume();
      throw new HTTPError(413, "payload_too_large", "request body exceeds 1 MiB");
    }
    chunks.push(chunk);
  }

  let value: unknown;
  try {
    value = JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new HTTPError(400, "invalid_request", "request body must be valid JSON");
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new HTTPError(400, "invalid_request", "request body must be a JSON object");
  }
  return value as Record<string, unknown>;
}

function requiredString(body: Record<string, unknown>, field: string): string {
  const value = body[field];
  if (typeof value !== "string" || value.trim() === "") {
    throw new HTTPError(400, "invalid_request", `${field} is required`);
  }
  return value.trim();
}

function optionalString(body: Record<string, unknown>, field: string): string | undefined {
  const value = body[field];
  if (value === undefined) {
    return undefined;
  }
  if (typeof value !== "string") {
    throw new HTTPError(400, "invalid_request", `${field} must be a string`);
  }
  return value;
}

function optionalPositiveInteger(
  body: Record<string, unknown>,
  field: string,
): number | undefined {
  const value = body[field];
  if (value === undefined) {
    return undefined;
  }
  if (!Number.isSafeInteger(value) || (value as number) <= 0) {
    throw new HTTPError(400, "invalid_request", `${field} must be a positive integer`);
  }
  return value as number;
}

function parseCursor(value: string | null): number {
  if (value === null) {
    return 0;
  }
  if (!/^(0|[1-9]\d*)$/.test(value)) {
    throw new HTTPError(400, "invalid_request", "after must be a non-negative integer");
  }
  const cursor = Number(value);
  if (!Number.isSafeInteger(cursor)) {
    throw new HTTPError(400, "invalid_request", "after must be a non-negative integer");
  }
  return cursor;
}

function decodePathPart(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    throw new HTTPError(400, "invalid_request", "path contains invalid encoding");
  }
}

function writeJSON(response: ServerResponse, status: number, body?: unknown): void {
  response.statusCode = status;
  response.setHeader("content-type", "application/json; charset=utf-8");
  if (body === undefined) {
    response.end();
    return;
  }
  response.end(JSON.stringify(body));
}

function writeError(response: ServerResponse, status: number, code: string, message: string): void {
  writeJSON(response, status, { error: { code, message } });
}

class HTTPError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

async function closeServer(server: Server): Promise<void> {
  if (!server.listening) {
    return;
  }
  await new Promise<void>((resolve, reject) => {
    server.close((error) => (error === undefined ? resolve() : reject(error)));
  });
}

async function removeSocketIfPresent(socketPath: string, rejectNonSocket: boolean): Promise<void> {
  try {
    const info = await lstat(socketPath);
    if (!info.isSocket()) {
      if (rejectNonSocket) {
        throw new Error(`refusing to replace non-socket path: ${socketPath}`);
      }
      return;
    }
    await unlink(socketPath);
  } catch (error) {
    if (isMissingPathError(error)) {
      return;
    }
    throw error;
  }
}

function isMissingPathError(error: unknown): boolean {
  return (
    error !== null &&
    typeof error === "object" &&
    "code" in error &&
    (error as { code?: unknown }).code === "ENOENT"
  );
}
