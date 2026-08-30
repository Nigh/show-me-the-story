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

export interface AuthPromptView {
  id: string;
  type: "text" | "secret" | "select" | "manual_code";
  message: string;
  placeholder?: string;
  options?: Array<{ id: string; label: string; description?: string }>;
}

export interface AuthEventView {
  cursor: number;
  type: "info" | "auth_url" | "device_code" | "progress";
  message?: string;
  url?: string;
  code?: string;
  links?: Array<{ url: string; label?: string }>;
  interval_seconds?: number;
  expires_in_seconds?: number;
}

export interface LoginSnapshot {
  id: string;
  provider_id: string;
  auth_type: AuthType;
  state: "running" | "waiting" | "succeeded" | "failed" | "cancelled";
  prompt?: AuthPromptView;
  events: AuthEventView[];
  next_cursor: number;
  error?: string;
}

export interface LoginStartRequest {
  provider_id: string;
  auth_type: AuthType;
}

export interface LoginResponse {
  prompt_id: string;
  value: string;
}

export interface LogoutRequest {
  provider_id: string;
}

export interface SessionSummary {
  project_id: string;
  session_id: string;
  message_count: number;
  restored: boolean;
}

export interface SessionSelectRequest {
  project_name: string;
}
