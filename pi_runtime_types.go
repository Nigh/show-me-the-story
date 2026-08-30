package main

import "context"

type PiRuntime interface {
	Health(context.Context) (PiHealth, error)
	Providers(context.Context) ([]PiProvider, error)
	Credentials(context.Context) ([]PiCredential, error)
	StartLogin(context.Context, PiLoginStartRequest) (PiLoginSnapshot, error)
	LoginStatus(context.Context, string, int) (PiLoginSnapshot, error)
	RespondLogin(context.Context, string, PiLoginResponse) error
	CancelLogin(context.Context, string) error
	Logout(context.Context, string) error
	ImportLegacy(context.Context, PiLegacyImportRequest) (PiLegacyImportResult, error)
	SelectProject(context.Context, string) (PiSessionSummary, error)
	CurrentSession(context.Context) (*PiSessionSummary, error)
	Close() error
}

type PiHealth struct {
	Status          string `json:"status"`
	ProtocolVersion int    `json:"protocol_version"`
	PiVersion       string `json:"pi_version"`
}

type PiModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ProviderID    string `json:"provider_id"`
	Reasoning     bool   `json:"reasoning"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
}

type PiProvider struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	AuthTypes  []string  `json:"auth_types"`
	Configured bool      `json:"configured"`
	Models     []PiModel `json:"models"`
}

type PiCredential struct {
	ProviderID string `json:"provider_id"`
	Type       string `json:"type"`
}

type PiLoginStartRequest struct {
	ProviderID string `json:"provider_id"`
	AuthType   string `json:"auth_type"`
}

type PiLoginResponse struct {
	PromptID string `json:"prompt_id"`
	Value    string `json:"value"`
}

type PiAuthOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type PiAuthPrompt struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Message     string         `json:"message"`
	Placeholder string         `json:"placeholder,omitempty"`
	Options     []PiAuthOption `json:"options,omitempty"`
}

type PiAuthLink struct {
	URL   string `json:"url"`
	Label string `json:"label,omitempty"`
}

type PiAuthEvent struct {
	Cursor           int          `json:"cursor"`
	Type             string       `json:"type"`
	Message          string       `json:"message,omitempty"`
	URL              string       `json:"url,omitempty"`
	Code             string       `json:"code,omitempty"`
	Links            []PiAuthLink `json:"links,omitempty"`
	IntervalSeconds  int          `json:"interval_seconds,omitempty"`
	ExpiresInSeconds int          `json:"expires_in_seconds,omitempty"`
}

type PiLoginSnapshot struct {
	ID         string        `json:"id"`
	ProviderID string        `json:"provider_id"`
	AuthType   string        `json:"auth_type"`
	State      string        `json:"state"`
	Prompt     *PiAuthPrompt `json:"prompt,omitempty"`
	Events     []PiAuthEvent `json:"events"`
	NextCursor int           `json:"next_cursor"`
	Error      string        `json:"error,omitempty"`
}

type PiLegacyImportRequest struct {
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

type PiLegacyImportResult struct {
	ProviderID         string `json:"provider_id"`
	ModelID            string `json:"model_id"`
	CredentialImported bool   `json:"credential_imported"`
	Available          bool   `json:"available"`
	MadeDefault        bool   `json:"made_default"`
}

type PiSessionSummary struct {
	ProjectID    string `json:"project_id"`
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
	Restored     bool   `json:"restored"`
}

type PiRuntimeError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *PiRuntimeError) Error() string {
	return "pi runtime: " + e.Code + ": " + e.Message
}
