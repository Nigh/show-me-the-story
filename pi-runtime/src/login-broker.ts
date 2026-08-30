import { randomUUID } from "node:crypto";
import type {
  AuthEvent,
  AuthInteraction,
  AuthPrompt,
  AuthType as PiAuthType,
} from "@earendil-works/pi-ai";
import type {
  AuthEventView,
  AuthPromptView,
  AuthType,
  LoginSnapshot,
} from "./protocol.js";

const maximumEvents = 200;
const maximumEventBytes = 4 * 1024;
const maximumResponseBytes = 64 * 1024;
const terminalLifetimeMs = 10 * 60 * 1_000;

export interface LoginRuntimePort {
  login(providerId: string, type: PiAuthType, interaction: AuthInteraction): Promise<unknown>;
}

export class LoginNotFoundError extends Error {
  readonly code = "login_not_found";

  constructor() {
    super("Login session not found");
    this.name = "LoginNotFoundError";
  }
}

export class LoginConflictError extends Error {
  readonly code = "login_conflict";

  constructor(message: string) {
    super(message);
    this.name = "LoginConflictError";
  }
}

export class LoginValidationError extends Error {
  readonly code = "login_validation";

  constructor(message: string) {
    super(message);
    this.name = "LoginValidationError";
  }
}

interface PendingResponse {
  promptId: string;
  resolve(value: string): void;
  reject(error: Error): void;
  cleanup(): void;
}

interface LoginRecord {
  id: string;
  providerId: string;
  authType: AuthType;
  state: LoginSnapshot["state"];
  prompt?: AuthPromptView;
  events: AuthEventView[];
  nextCursor: number;
  error?: string;
  controller: AbortController;
  pending?: PendingResponse;
  terminalAt?: number;
}

export class LoginBroker {
  private readonly logins = new Map<string, LoginRecord>();

  constructor(
    private readonly runtime: LoginRuntimePort,
    private readonly clock: () => number = Date.now,
  ) {}

  start(providerId: string, authType: AuthType): LoginSnapshot {
    const trimmedProviderId = providerId.trim();
    if (trimmedProviderId === "") {
      throw new LoginValidationError("Provider id is required");
    }
    const login: LoginRecord = {
      id: randomUUID(),
      providerId: trimmedProviderId,
      authType,
      state: "running",
      events: [],
      nextCursor: 0,
      controller: new AbortController(),
    };
    this.logins.set(login.id, login);
    void this.run(login);
    return this.snapshot(login.id, 0);
  }

  snapshot(id: string, afterCursor: number): LoginSnapshot {
    const login = this.requireLogin(id);
    if (!Number.isInteger(afterCursor) || afterCursor < 0) {
      throw new LoginValidationError("Event cursor must be a non-negative integer");
    }
    return {
      id: login.id,
      provider_id: login.providerId,
      auth_type: login.authType,
      state: login.state,
      ...(login.prompt ? { prompt: copyPrompt(login.prompt) } : {}),
      events: login.events
        .filter((event) => event.cursor > afterCursor)
        .map(copyEvent),
      next_cursor: login.nextCursor,
      ...(login.error ? { error: login.error } : {}),
    };
  }

  respond(id: string, promptId: string, value: string): void {
    const login = this.requireLogin(id);
    if (Buffer.byteLength(value, "utf8") > maximumResponseBytes) {
      throw new LoginValidationError("Authentication response is too large");
    }
    const pending = login.pending;
    if (!pending || pending.promptId !== promptId) {
      throw new LoginConflictError("Authentication prompt is no longer pending");
    }
    pending.cleanup();
    login.pending = undefined;
    login.prompt = undefined;
    login.state = "running";
    pending.resolve(value);
  }

  cancel(id: string): void {
    const login = this.requireLogin(id);
    if (isTerminal(login.state)) {
      return;
    }
    login.state = "cancelled";
    login.terminalAt = this.clock();
    login.prompt = undefined;
    login.controller.abort();
  }

  sweepExpired(): void {
    const now = this.clock();
    for (const [id, login] of this.logins) {
      if (login.terminalAt !== undefined && now - login.terminalAt >= terminalLifetimeMs) {
        this.logins.delete(id);
      }
    }
  }

  dispose(): void {
    for (const login of this.logins.values()) {
      login.controller.abort();
      login.pending?.cleanup();
      login.pending?.reject(abortError());
      login.pending = undefined;
      login.prompt = undefined;
    }
    this.logins.clear();
  }

  private async run(login: LoginRecord): Promise<void> {
    try {
      await this.runtime.login(login.providerId, login.authType, {
        signal: login.controller.signal,
        prompt: (prompt) => this.waitForResponse(login, prompt),
        notify: (event) => this.appendEvent(login, event),
      });
      if (login.controller.signal.aborted) {
        this.finish(login, "cancelled");
      } else {
        this.finish(login, "succeeded");
      }
    } catch {
      if (login.controller.signal.aborted) {
        this.finish(login, "cancelled");
      } else {
        login.error = "Authentication failed";
        this.finish(login, "failed");
      }
    }
  }

  private waitForResponse(login: LoginRecord, prompt: AuthPrompt): Promise<string> {
    if (login.controller.signal.aborted || prompt.signal?.aborted) {
      return Promise.reject(abortError());
    }
    if (login.pending) {
      return Promise.reject(new LoginConflictError("Another authentication prompt is pending"));
    }

    const promptId = randomUUID();
    login.prompt = promptView(promptId, prompt);
    login.state = "waiting";
    return new Promise<string>((resolve, reject) => {
      const onAbort = () => {
        const pending = login.pending;
        if (!pending || pending.promptId !== promptId) {
          return;
        }
        pending.cleanup();
        login.pending = undefined;
        login.prompt = undefined;
        reject(abortError());
      };
      const cleanup = () => {
        login.controller.signal.removeEventListener("abort", onAbort);
        prompt.signal?.removeEventListener("abort", onAbort);
      };
      login.pending = { promptId, resolve, reject, cleanup };
      login.controller.signal.addEventListener("abort", onAbort, { once: true });
      prompt.signal?.addEventListener("abort", onAbort, { once: true });
    });
  }

  private appendEvent(login: LoginRecord, event: AuthEvent): void {
    login.nextCursor += 1;
    login.events.push(eventView(login.nextCursor, event));
    if (login.events.length > maximumEvents) {
      login.events.splice(0, login.events.length - maximumEvents);
    }
  }

  private finish(login: LoginRecord, state: "succeeded" | "failed" | "cancelled"): void {
    login.pending?.cleanup();
    login.pending = undefined;
    login.prompt = undefined;
    login.state = state;
    login.terminalAt = this.clock();
  }

  private requireLogin(id: string): LoginRecord {
    const login = this.logins.get(id);
    if (!login) {
      throw new LoginNotFoundError();
    }
    return login;
  }
}

function promptView(id: string, prompt: AuthPrompt): AuthPromptView {
  return {
    id,
    type: prompt.type,
    message: safeText(prompt.message),
    ...(prompt.type !== "select" && prompt.placeholder
      ? { placeholder: safeText(prompt.placeholder) }
      : {}),
    ...(prompt.type === "select"
      ? {
          options: prompt.options.map((option) => ({
            id: safeText(option.id),
            label: safeText(option.label),
            ...(option.description ? { description: safeText(option.description) } : {}),
          })),
        }
      : {}),
  };
}

function eventView(cursor: number, event: AuthEvent): AuthEventView {
  switch (event.type) {
    case "info":
      return {
        cursor,
        type: event.type,
        message: safeText(event.message),
        ...(event.links
          ? {
              links: event.links.map((link) => ({
                url: safeText(link.url),
                ...(link.label ? { label: safeText(link.label) } : {}),
              })),
            }
          : {}),
      };
    case "auth_url":
      return {
        cursor,
        type: event.type,
        url: safeText(event.url),
        ...(event.instructions ? { message: safeText(event.instructions) } : {}),
      };
    case "device_code":
      return {
        cursor,
        type: event.type,
        code: safeText(event.userCode),
        url: safeText(event.verificationUri),
        ...(event.intervalSeconds !== undefined
          ? { interval_seconds: event.intervalSeconds }
          : {}),
        ...(event.expiresInSeconds !== undefined
          ? { expires_in_seconds: event.expiresInSeconds }
          : {}),
      };
    case "progress":
      return { cursor, type: event.type, message: safeText(event.message) };
  }
}

function safeText(value: string): string {
  const redacted = value
    .replace(/Bearer\s+[^\s]+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_-]{8,}\b/g, "[redacted]")
    .replace(/([?&](?:access_token|api_key)=)[^&\s]+/gi, "$1[redacted]");
  const encoded = Buffer.from(redacted, "utf8");
  return encoded.length <= maximumEventBytes
    ? redacted
    : encoded.subarray(0, maximumEventBytes).toString("utf8");
}

function copyPrompt(prompt: AuthPromptView): AuthPromptView {
  return {
    ...prompt,
    ...(prompt.options ? { options: prompt.options.map((option) => ({ ...option })) } : {}),
  };
}

function copyEvent(event: AuthEventView): AuthEventView {
  return {
    ...event,
    ...(event.links ? { links: event.links.map((link) => ({ ...link })) } : {}),
  };
}

function isTerminal(state: LoginSnapshot["state"]): boolean {
  return state === "succeeded" || state === "failed" || state === "cancelled";
}

function abortError(): Error {
  const error = new Error("Authentication cancelled");
  error.name = "AbortError";
  return error;
}
