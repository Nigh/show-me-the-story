import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SettingsManager } from "@earendil-works/pi-coding-agent";
import { describe, expect, it } from "vitest";
import { writeJsonAtomic } from "./files.js";
import { createPiModelRuntime, ModelService } from "./models.js";
import { ensureRuntimePaths, projectId, resolveRuntimePaths } from "./paths.js";
import {
  PiStorySessionFactory,
  StorySessionRegistry,
  type SessionHandle,
  type StorySessionFactory,
} from "./sessions.js";

class FakeSession implements SessionHandle {
  disposed = false;

  constructor(
    readonly projectId: string,
    readonly sessionId: string,
    readonly messages = 0,
  ) {}

  summary(restored: boolean) {
    return {
      project_id: this.projectId,
      session_id: this.sessionId,
      message_count: this.messages,
      restored,
    };
  }

  dispose() {
    this.disposed = true;
  }
}

class FakeFactory implements StorySessionFactory {
  sessions: FakeSession[] = [];
  failures = new Set<string>();
  delays = new Map<string, Promise<void>>();

  async open(projectName: string) {
    await this.delays.get(projectName);
    if (this.failures.has(projectName)) {
      throw new Error(`cannot open ${projectName}`);
    }
    const id = projectId(projectName);
    const session = new FakeSession(id, `session-${this.sessions.length + 1}`);
    const restored = this.sessions.some((item) => item.projectId === id);
    this.sessions.push(session);
    return { handle: session, restored };
  }
}

describe("StorySessionRegistry", () => {
  it("uses the same isolated project for NFC-equivalent names", async () => {
    const factory = new FakeFactory();
    const registry = new StorySessionRegistry(factory);

    const first = await registry.select("Cafe\u0301");
    const second = await registry.select("Caf\u00e9");

    expect(first.project_id).toBe(projectId("Caf\u00e9"));
    expect(second).toEqual(first);
    expect(factory.sessions).toHaveLength(1);
    expect(factory.sessions[0]?.disposed).toBe(false);
  });

  it("opens a replacement before disposing the previous project", async () => {
    const factory = new FakeFactory();
    const registry = new StorySessionRegistry(factory);

    await registry.select("first");
    const first = factory.sessions[0]!;
    const second = await registry.select("second");

    expect(second.project_id).toBe(projectId("second"));
    expect(first.disposed).toBe(true);
    expect(factory.sessions[1]?.disposed).toBe(false);
  });

  it("keeps the previous project active when replacement fails", async () => {
    const factory = new FakeFactory();
    const registry = new StorySessionRegistry(factory);
    const first = await registry.select("first");
    factory.failures.add("broken");

    await expect(registry.select("broken")).rejects.toThrow("cannot open broken");

    expect(registry.current()).toEqual(first);
    expect(factory.sessions[0]?.disposed).toBe(false);
  });

  it("serializes concurrent project selection", async () => {
    const factory = new FakeFactory();
    let releaseFirst!: () => void;
    factory.delays.set(
      "first",
      new Promise<void>((resolve) => {
        releaseFirst = resolve;
      }),
    );
    const registry = new StorySessionRegistry(factory);

    const firstSelection = registry.select("first");
    const secondSelection = registry.select("second");
    await Promise.resolve();
    expect(factory.sessions).toHaveLength(0);

    releaseFirst();
    await expect(firstSelection).resolves.toMatchObject({ project_id: projectId("first") });
    await expect(secondSelection).resolves.toMatchObject({ project_id: projectId("second") });
    expect(factory.sessions.map((item) => item.projectId)).toEqual([
      projectId("first"),
      projectId("second"),
    ]);
    expect(factory.sessions[0]?.disposed).toBe(true);
  });

  it("returns detached current-session summaries and disposes the active session", async () => {
    const factory = new FakeFactory();
    const registry = new StorySessionRegistry(factory);
    await registry.select("first");

    const view = registry.current()!;
    view.session_id = "changed-by-caller";

    expect(registry.current()?.session_id).toBe("session-1");
    registry.dispose();
    expect(factory.sessions[0]?.disposed).toBe(true);
    expect(registry.current()).toBeUndefined();
  });

  it("rejects and disposes a session that finishes opening after shutdown", async () => {
    const factory = new FakeFactory();
    let release!: () => void;
    factory.delays.set(
      "late",
      new Promise<void>((resolve) => {
        release = resolve;
      }),
    );
    const registry = new StorySessionRegistry(factory);

    const selection = registry.select("late");
    await Promise.resolve();
    registry.dispose();
    release();

    await expect(selection).rejects.toThrow("closed");
    expect(factory.sessions[0]?.disposed).toBe(true);
    expect(registry.current()).toBeUndefined();
  });
});

describe("PiStorySessionFactory", () => {
  it("persists a Pi session per project with every tool and resource source disabled", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-sessions-"));
    const paths = resolveRuntimePaths(root);
    await ensureRuntimePaths(paths);
    await writeJsonAtomic(paths.modelsPath, {
      providers: {
        local: {
          name: "Local fixture",
          baseUrl: "http://127.0.0.1:11434/v1",
          api: "openai-completions",
          authHeader: true,
          models: [{ id: "fixture-model", name: "fixture-model" }],
        },
      },
    });
    const settings = SettingsManager.create(paths.root, paths.agentDir, { projectTrusted: true });
    const runtime = await createPiModelRuntime(paths);
    const models = new ModelService(runtime, settings, paths);
    await models.importLegacy({
      base_url: "http://127.0.0.1:11434/v1",
      model: "fixture-model",
      api_key: "fixture-key",
    });
    const factory = new PiStorySessionFactory(paths, runtime);

    const first = await factory.open("Persistent Story");
    const firstSummary = first.handle.summary(first.restored);
    expect(first.restored).toBe(false);
    expect(firstSummary).toMatchObject({
      project_id: projectId("Persistent Story"),
      message_count: 0,
    });
    expect(firstSummary.session_id).not.toBe("");
    first.handle.dispose();

    const second = await factory.open("Persistent Story");
    expect(second.restored).toBe(true);
    expect(second.handle.summary(second.restored)).toEqual({
      ...firstSummary,
      restored: true,
    });
    second.handle.dispose();
  });
});
