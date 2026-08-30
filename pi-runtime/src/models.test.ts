import { mkdtemp, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SettingsManager } from "@earendil-works/pi-coding-agent";
import type { AuthInteraction, AuthType as PiAuthType } from "@earendil-works/pi-ai";
import { describe, expect, it } from "vitest";
import { writeJsonAtomic } from "./files.js";
import {
  createPiModelRuntime,
  ModelService,
  type ModelRuntimePort,
  type ModelSettingsPort,
} from "./models.js";
import { ensureRuntimePaths, resolveRuntimePaths } from "./paths.js";

const provider = {
  id: "subscription",
  name: "Subscription Provider",
  auth: {
    apiKey: { login: async () => ({ type: "api_key" as const }) },
    oauth: { login: async () => ({ type: "oauth" as const }) },
  },
};

const model = {
  id: "reasoning-model",
  name: "Reasoning Model",
  provider: "subscription",
  reasoning: true,
  contextWindow: 200_000,
  maxTokens: 32_000,
};

class FakeRuntime implements ModelRuntimePort {
  providers = [provider];
  models = [model];
  credentials = [{ providerId: "subscription", type: "oauth" as const }];
  configured = new Set(["subscription"]);
  availableModels = [model];
  loginValues: string[] = [];
  logoutValues: string[] = [];
  refreshCount = 0;

  getProviders() {
    return this.providers;
  }

  getModels(providerId?: string) {
    return providerId ? this.models.filter((item) => item.provider === providerId) : this.models;
  }

  async checkAuth(providerId: string) {
    return this.configured.has(providerId) ? { type: "api_key" } : undefined;
  }

  async getAvailable(providerId?: string) {
    return providerId
      ? this.availableModels.filter((item) => item.provider === providerId)
      : this.availableModels;
  }

  async listCredentials() {
    return this.credentials;
  }

  async login(_providerId: string, _type: PiAuthType, interaction: AuthInteraction) {
    const value = await interaction.prompt({ type: "secret", message: "API key" });
    this.loginValues.push(value);
    this.availableModels = [
      ...this.availableModels,
      {
        id: "legacy-model",
        name: "legacy-model",
        provider: "show-me-the-story-legacy",
        reasoning: false,
        contextWindow: 128_000,
        maxTokens: 8_192,
      },
    ];
    return { type: "api_key" as const, key: value };
  }

  async logout(providerId: string) {
    this.logoutValues.push(providerId);
  }

  async refresh() {
    this.refreshCount += 1;
  }
}

class FakeSettings implements ModelSettingsPort {
  defaults: Array<{ provider: string; model: string }> = [];
  flushCount = 0;

  setDefaultModelAndProvider(providerId: string, modelId: string) {
    this.defaults.push({ provider: providerId, model: modelId });
  }

  async flush() {
    this.flushCount += 1;
  }
}

async function testService() {
  const root = await mkdtemp(join(tmpdir(), "story-pi-models-"));
  const paths = resolveRuntimePaths(root);
  await ensureRuntimePaths(paths);
  const runtime = new FakeRuntime();
  const settings = new FakeSettings();
  return { paths, runtime, settings, service: new ModelService(runtime, settings, paths) };
}

describe("ModelService", () => {
  it("returns a secret-free provider, model, and credential catalog", async () => {
    const { service } = await testService();

    await expect(service.listProviders()).resolves.toEqual([
      {
        id: "subscription",
        name: "Subscription Provider",
        auth_types: ["api_key", "oauth"],
        configured: true,
        models: [
          {
            id: "reasoning-model",
            name: "Reasoning Model",
            provider_id: "subscription",
            reasoning: true,
            context_window: 200_000,
            max_tokens: 32_000,
          },
        ],
      },
    ]);
    await expect(service.listCredentials()).resolves.toEqual([
      { provider_id: "subscription", type: "oauth" },
    ]);
  });

  it("merges a legacy provider, persists its key through Pi, and activates an available model", async () => {
    const { paths, runtime, settings, service } = await testService();
    await writeJsonAtomic(paths.modelsPath, {
      providers: { existing: { name: "Existing", baseUrl: "http://localhost/v1" } },
    });

    const result = await service.importLegacy({
      base_url: "https://example.test/v1",
      model: "legacy-model",
      api_key: "secret-value",
      max_tokens: 16_384,
    });

    expect(result).toEqual({
      provider_id: "show-me-the-story-legacy",
      model_id: "legacy-model",
      credential_imported: true,
      available: true,
      made_default: true,
    });
    const stored = JSON.parse(await readFile(paths.modelsPath, "utf8"));
    expect(stored.providers.existing.name).toBe("Existing");
    expect(stored.providers["show-me-the-story-legacy"]).toEqual({
      name: "Imported show-me-the-story API",
      baseUrl: "https://example.test/v1",
      api: "openai-completions",
      authHeader: true,
      models: [{ id: "legacy-model", name: "legacy-model", maxTokens: 16_384 }],
    });
    expect(await readFile(paths.modelsPath, "utf8")).not.toContain("secret-value");
    expect(JSON.stringify(result)).not.toContain("secret-value");
    expect(runtime.loginValues).toEqual(["secret-value"]);
    expect(runtime.refreshCount).toBe(1);
    expect(settings.defaults).toEqual([
      { provider: "show-me-the-story-legacy", model: "legacy-model" },
    ]);
    expect(settings.flushCount).toBe(1);
  });

  it("imports configuration without replacing defaults when auth is unavailable", async () => {
    const { runtime, settings, service } = await testService();
    runtime.availableModels = [];

    await expect(
      service.importLegacy({ base_url: "http://localhost:11434/v1", model: "local-model" }),
    ).resolves.toEqual({
      provider_id: "show-me-the-story-legacy",
      model_id: "local-model",
      credential_imported: false,
      available: false,
      made_default: false,
    });
    expect(runtime.loginValues).toEqual([]);
    expect(settings.defaults).toEqual([]);
    expect(settings.flushCount).toBe(0);
  });

  it.each([
    [{ base_url: "file:///tmp/model", model: "model" }, "HTTP"],
    [{ base_url: "https://example.test/v1", model: "   " }, "model"],
  ])("rejects an invalid legacy mapping", async (input, message) => {
    const { service } = await testService();
    await expect(service.importLegacy(input)).rejects.toThrow(message);
  });

  it("loads the imported provider through the real Pi runtime without network access", async () => {
    const root = await mkdtemp(join(tmpdir(), "story-pi-real-models-"));
    const paths = resolveRuntimePaths(root);
    await ensureRuntimePaths(paths);
    const settings = SettingsManager.create(paths.root, paths.agentDir, { projectTrusted: true });
    const runtime = await createPiModelRuntime(paths);
    const service = new ModelService(runtime, settings, paths);

    await expect(
      service.importLegacy({
        base_url: "http://127.0.0.1:11434/v1",
        model: "fixture-model",
        api_key: "fixture-key",
      }),
    ).resolves.toEqual({
      provider_id: "show-me-the-story-legacy",
      model_id: "fixture-model",
      credential_imported: true,
      available: true,
      made_default: true,
    });
    expect(settings.getDefaultProvider()).toBe("show-me-the-story-legacy");
    expect(settings.getDefaultModel()).toBe("fixture-model");
    expect((await stat(paths.authPath)).mode & 0o777).toBe(0o600);
    expect(await readFile(paths.modelsPath, "utf8")).not.toContain("fixture-key");
  });
});
