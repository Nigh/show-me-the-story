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
