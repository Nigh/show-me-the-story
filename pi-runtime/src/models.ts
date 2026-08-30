import { ModelRuntime, type SettingsManager } from "@earendil-works/pi-coding-agent";
import type { AuthInteraction, AuthType as PiAuthType } from "@earendil-works/pi-ai";
import { readJsonIfExists, writeJsonAtomic } from "./files.js";
import type { RuntimePaths } from "./paths.js";
import type {
  AuthType,
  CredentialSummary,
  LegacyAPIImport,
  LegacyImportResult,
  ModelSummary,
  ProviderSummary,
} from "./protocol.js";

const legacyProviderId = "show-me-the-story-legacy" as const;

interface RuntimeProviderView {
  id: string;
  name: string;
  auth: {
    apiKey?: { login?: unknown };
    oauth?: { login?: unknown };
  };
}

interface RuntimeModelView {
  id: string;
  name: string;
  provider: string;
  reasoning: boolean;
  contextWindow: number;
  maxTokens: number;
}

interface RuntimeCredentialView {
  providerId: string;
  type: AuthType;
}

export interface ModelRuntimePort {
  getProviders(): readonly RuntimeProviderView[];
  getModels(providerId?: string): readonly RuntimeModelView[];
  checkAuth(providerId: string): Promise<unknown | undefined>;
  getAvailable(providerId?: string): Promise<readonly RuntimeModelView[]>;
  listCredentials(): Promise<readonly RuntimeCredentialView[]>;
  login(providerId: string, type: PiAuthType, interaction: AuthInteraction): Promise<unknown>;
  logout(providerId: string): Promise<void>;
  refresh(): Promise<unknown>;
}

export interface ModelSettingsPort {
  setDefaultModelAndProvider(providerId: string, modelId: string): void;
  flush(): Promise<void>;
}

export interface ModelCatalog {
  listProviders(): Promise<ProviderSummary[]>;
  listCredentials(): Promise<CredentialSummary[]>;
  importLegacy(input: LegacyAPIImport): Promise<LegacyImportResult>;
  logout(providerId: string): Promise<void>;
}

export async function createPiModelRuntime(paths: RuntimePaths): Promise<ModelRuntime> {
  return ModelRuntime.create({
    authPath: paths.authPath,
    modelsPath: paths.modelsPath,
    modelsStorePath: paths.modelsStorePath,
    allowModelNetwork: false,
  });
}

export class ModelService implements ModelCatalog {
  constructor(
    private readonly runtime: ModelRuntimePort,
    private readonly settings: ModelSettingsPort,
    private readonly paths: RuntimePaths,
  ) {}

  static async create(paths: RuntimePaths, settings: SettingsManager): Promise<ModelService> {
    return new ModelService(await createPiModelRuntime(paths), settings, paths);
  }

  async listProviders(): Promise<ProviderSummary[]> {
    return Promise.all(
      this.runtime.getProviders().map(async (provider) => ({
        id: provider.id,
        name: provider.name,
        auth_types: authTypes(provider),
        configured: (await this.runtime.checkAuth(provider.id)) !== undefined,
        models: this.runtime.getModels(provider.id).map(modelSummary),
      })),
    );
  }

  async listCredentials(): Promise<CredentialSummary[]> {
    return (await this.runtime.listCredentials()).map((credential) => ({
      provider_id: credential.providerId,
      type: credential.type,
    }));
  }

  async importLegacy(input: LegacyAPIImport): Promise<LegacyImportResult> {
    const baseUrl = validateBaseURL(input.base_url);
    const modelId = input.model.trim();
    if (modelId === "") {
      throw new Error("legacy API model is required");
    }

    const current = await readJsonIfExists<Record<string, unknown>>(this.paths.modelsPath);
    if (current !== undefined && !isRecord(current)) {
      throw new Error("models.json root must be an object");
    }
    const providers = isRecord(current?.providers) ? current.providers : {};
    const modelConfig: Record<string, unknown> = { id: modelId, name: modelId };
    if (
      input.max_tokens !== undefined &&
      Number.isInteger(input.max_tokens) &&
      input.max_tokens > 0
    ) {
      modelConfig.maxTokens = input.max_tokens;
    }

    await writeJsonAtomic(this.paths.modelsPath, {
      ...(current ?? {}),
      providers: {
        ...providers,
        [legacyProviderId]: {
          name: "Imported show-me-the-story API",
          baseUrl,
          api: "openai-completions",
          authHeader: true,
          models: [modelConfig],
        },
      },
    });
    await this.runtime.refresh();

    const credentialImported = input.api_key !== undefined && input.api_key !== "";
    if (credentialImported) {
      await this.runtime.login(legacyProviderId, "api_key", {
        prompt: async (prompt) => {
          if (prompt.type !== "secret") {
            throw new Error("legacy import expected a secret prompt");
          }
          return input.api_key!;
        },
        notify: () => undefined,
      });
    }

    const available = (await this.runtime.getAvailable(legacyProviderId)).some(
      (model) => model.provider === legacyProviderId && model.id === modelId,
    );
    if (available) {
      this.settings.setDefaultModelAndProvider(legacyProviderId, modelId);
      await this.settings.flush();
    }

    return {
      provider_id: legacyProviderId,
      model_id: modelId,
      credential_imported: credentialImported,
      available,
      made_default: available,
    };
  }

  async logout(providerId: string): Promise<void> {
    const trimmed = providerId.trim();
    if (trimmed === "") {
      throw new Error("provider id is required");
    }
    await this.runtime.logout(trimmed);
  }
}

function authTypes(provider: RuntimeProviderView): AuthType[] {
  const types: AuthType[] = [];
  if (typeof provider.auth.apiKey?.login === "function") {
    types.push("api_key");
  }
  if (typeof provider.auth.oauth?.login === "function") {
    types.push("oauth");
  }
  return types;
}

function modelSummary(model: RuntimeModelView): ModelSummary {
  return {
    id: model.id,
    name: model.name,
    provider_id: model.provider,
    reasoning: model.reasoning,
    context_window: model.contextWindow,
    max_tokens: model.maxTokens,
  };
}

function validateBaseURL(value: string): string {
  const trimmed = value.trim();
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error("legacy API base URL must be HTTP(S)");
  }
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.host === "") {
    throw new Error("legacy API base URL must be HTTP(S)");
  }
  return trimmed;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
