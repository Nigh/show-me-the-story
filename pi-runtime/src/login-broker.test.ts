import type { AuthInteraction, AuthType as PiAuthType } from "@earendil-works/pi-ai";
import { describe, expect, it } from "vitest";
import {
  LoginBroker,
  LoginConflictError,
  LoginNotFoundError,
  LoginValidationError,
  type LoginRuntimePort,
} from "./login-broker.js";

class PromptingRuntime implements LoginRuntimePort {
  async login(_providerId: string, _type: PiAuthType, interaction: AuthInteraction) {
    interaction.notify({
      type: "auth_url",
      url: "https://login.example.test/authorize",
      instructions: "Open the authorization page",
    });
    interaction.notify({
      type: "device_code",
      userCode: "ABCD-EFGH",
      verificationUri: "https://login.example.test/device",
      intervalSeconds: 5,
      expiresInSeconds: 600,
    });
    const value = await interaction.prompt({
      type: "secret",
      message: "API key",
      placeholder: "sk-…",
    });
    return { type: "api_key" as const, key: value };
  }
}

async function waitForState(
  broker: LoginBroker,
  id: string,
  state: "waiting" | "succeeded" | "failed" | "cancelled",
) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    const snapshot = broker.snapshot(id, 0);
    if (snapshot.state === state) {
      return snapshot;
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`login ${id} did not reach ${state}`);
}

describe("LoginBroker", () => {
  it("bridges Pi events and prompts without retaining the submitted secret", async () => {
    const broker = new LoginBroker(new PromptingRuntime());
    const started = broker.start("subscription", "oauth");
    expect(started.id).toMatch(/^[0-9a-f-]{36}$/);

    const waiting = await waitForState(broker, started.id, "waiting");
    expect(waiting.events).toEqual([
      {
        cursor: 1,
        type: "auth_url",
        url: "https://login.example.test/authorize",
        message: "Open the authorization page",
      },
      {
        cursor: 2,
        type: "device_code",
        code: "ABCD-EFGH",
        url: "https://login.example.test/device",
        interval_seconds: 5,
        expires_in_seconds: 600,
      },
    ]);
    expect(waiting.prompt).toMatchObject({ type: "secret", message: "API key" });
    expect(waiting.next_cursor).toBe(2);

    broker.respond(started.id, waiting.prompt!.id, "secret-value");
    const succeeded = await waitForState(broker, started.id, "succeeded");
    expect(succeeded.prompt).toBeUndefined();
    expect(JSON.stringify(succeeded)).not.toContain("secret-value");
    expect(broker.snapshot(started.id, 1).events).toHaveLength(1);
  });

  it("rejects invalid, duplicate, and oversized prompt responses", async () => {
    const broker = new LoginBroker(new PromptingRuntime());
    const started = broker.start("subscription", "api_key");
    const waiting = await waitForState(broker, started.id, "waiting");

    expect(() => broker.respond(started.id, "wrong", "value")).toThrow(LoginConflictError);
    expect(() => broker.respond(started.id, waiting.prompt!.id, "x".repeat(65_537))).toThrow(
      LoginValidationError,
    );
    broker.respond(started.id, waiting.prompt!.id, "accepted");
    expect(() => broker.respond(started.id, waiting.prompt!.id, "again")).toThrow(
      LoginConflictError,
    );
    await waitForState(broker, started.id, "succeeded");
  });

  it("cancels a pending Pi login", async () => {
    const broker = new LoginBroker(new PromptingRuntime());
    const started = broker.start("subscription", "oauth");
    await waitForState(broker, started.id, "waiting");

    broker.cancel(started.id);

    const cancelled = await waitForState(broker, started.id, "cancelled");
    expect(cancelled.prompt).toBeUndefined();
  });

  it("sanitizes provider failures", async () => {
    const runtime: LoginRuntimePort = {
      async login() {
        throw new Error("provider rejected secret-value");
      },
    };
    const broker = new LoginBroker(runtime);
    const started = broker.start("subscription", "oauth");

    const failed = await waitForState(broker, started.id, "failed");
    expect(failed.error).toBe("Authentication failed");
    expect(JSON.stringify(failed)).not.toContain("secret-value");
  });

  it("bounds event history and terminal-session lifetime", async () => {
    let now = 1_000;
    const runtime: LoginRuntimePort = {
      async login(_providerId, _type, interaction) {
        for (let index = 0; index < 205; index += 1) {
          interaction.notify({ type: "progress", message: `step ${index}` });
        }
        return { type: "api_key" as const, key: "fixture" };
      },
    };
    const broker = new LoginBroker(runtime, () => now);
    const started = broker.start("subscription", "api_key");
    const succeeded = await waitForState(broker, started.id, "succeeded");
    expect(succeeded.events).toHaveLength(200);
    expect(succeeded.events[0]?.cursor).toBe(6);
    expect(succeeded.next_cursor).toBe(205);

    now += 10 * 60 * 1_000 - 1;
    broker.sweepExpired();
    expect(broker.snapshot(started.id, 0).state).toBe("succeeded");
    now += 1;
    broker.sweepExpired();
    expect(() => broker.snapshot(started.id, 0)).toThrow(LoginNotFoundError);
  });

  it("disposes all active logins", async () => {
    const broker = new LoginBroker(new PromptingRuntime());
    const first = broker.start("first", "oauth");
    const second = broker.start("second", "oauth");
    await Promise.all([
      waitForState(broker, first.id, "waiting"),
      waitForState(broker, second.id, "waiting"),
    ]);

    broker.dispose();

    expect(() => broker.snapshot(first.id, 0)).toThrow(LoginNotFoundError);
    expect(() => broker.snapshot(second.id, 0)).toThrow(LoginNotFoundError);
  });
});
