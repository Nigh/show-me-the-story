package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const piRequestBodyLimit = 1024 * 1024

type piStatusProvider interface {
	Status() PiRuntimeStatus
}

type piLegacyImportPreview struct {
	ProviderID string `json:"provider_id"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	MaxTokens  int    `json:"max_tokens,omitempty"`
}

func (h *Handlers) GetPiStatus(w http.ResponseWriter, r *http.Request) {
	if h.piRuntime == nil {
		h.writeJSON(w, http.StatusOK, PiRuntimeStatus{Available: false, Reason: "unavailable"})
		return
	}
	if provider, ok := h.piRuntime.(piStatusProvider); ok {
		h.writeJSON(w, http.StatusOK, provider.Status())
		return
	}
	health, err := h.piRuntime.Health(r.Context())
	if err != nil {
		h.writeJSON(w, http.StatusOK, PiRuntimeStatus{Available: false, Reason: "unavailable"})
		return
	}
	h.writeJSON(w, http.StatusOK, PiRuntimeStatus{Available: true, Pi: health.PiVersion})
}

func (h *Handlers) GetPiProviders(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	providers, err := runtime.Providers(r.Context())
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, providers)
}

func (h *Handlers) GetPiCredentials(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	credentials, err := runtime.Credentials(r.Context())
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, credentials)
}

func (h *Handlers) PostPiLogin(w http.ResponseWriter, r *http.Request) {
	var request PiLoginStartRequest
	if err := decodePiJSON(w, r, &request); err != nil || strings.TrimSpace(request.ProviderID) == "" || (request.AuthType != "api_key" && request.AuthType != "oauth") {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_request")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	snapshot, err := runtime.StartLogin(r.Context(), request)
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusAccepted, snapshot)
}

func (h *Handlers) GetPiLogin(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	after, err := nonNegativeQueryInteger(r, "after")
	if id == "" || err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_request")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	snapshot, err := runtime.LoginStatus(r.Context(), id, after)
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handlers) PostPiLoginResponse(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var response PiLoginResponse
	if id == "" || decodePiJSON(w, r, &response) != nil || strings.TrimSpace(response.PromptID) == "" || response.Value == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_request")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	if err := runtime.RespondLogin(r.Context(), id, response); err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) DeletePiLogin(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_request")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	if err := runtime.CancelLogin(r.Context(), id); err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) PostPiLogout(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ProviderID string `json:"provider_id"`
	}
	if decodePiJSON(w, r, &request) != nil || strings.TrimSpace(request.ProviderID) == "" {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_request")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	if err := runtime.Logout(r.Context(), request.ProviderID); err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) PostPiLegacyImportPreview(w http.ResponseWriter, r *http.Request) {
	request, err := h.legacyImportRequest()
	if err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_legacy_config")
		return
	}
	h.writeJSON(w, http.StatusOK, piLegacyImportPreview{
		ProviderID: "show-me-the-story-legacy",
		BaseURL:    request.BaseURL,
		Model:      request.Model,
		MaxTokens:  request.MaxTokens,
	})
}

func (h *Handlers) PostPiLegacyImport(w http.ResponseWriter, r *http.Request) {
	request, err := h.legacyImportRequest()
	if err != nil {
		h.writeErrorReq(w, r, http.StatusBadRequest, "pi_invalid_legacy_config")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	result, err := runtime.ImportLegacy(r.Context(), request)
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) PostPiSessionSelect(w http.ResponseWriter, r *http.Request) {
	h.projectMu.RLock()
	projectName := h.projectName
	h.projectMu.RUnlock()
	if projectName == "" {
		h.writeErrorReq(w, r, http.StatusConflict, "select_project_first")
		return
	}
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	summary, err := runtime.SelectProject(r.Context(), projectName)
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, summary)
}

func (h *Handlers) GetPiSessionCurrent(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.requirePiRuntime(w, r)
	if !ok {
		return
	}
	summary, err := runtime.CurrentSession(r.Context())
	if err != nil {
		h.writePiRuntimeError(w, r, err)
		return
	}
	if summary == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.writeJSON(w, http.StatusOK, summary)
}

func (h *Handlers) legacyImportRequest() (PiLegacyImportRequest, error) {
	baseURL := resolveAPIBase(h.apiCfg.BaseURL, h.apiCfg.URLStrict)
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return PiLegacyImportRequest{}, errors.New("legacy API base URL must be HTTP(S)")
	}
	if strings.TrimSpace(h.apiCfg.Model) == "" {
		return PiLegacyImportRequest{}, errors.New("legacy API model is required")
	}
	return PiLegacyImportRequest{
		BaseURL:   baseURL,
		Model:     h.apiCfg.Model,
		APIKey:    h.apiCfg.APIKey,
		MaxTokens: h.apiCfg.MaxTokens,
	}, nil
}

func (h *Handlers) requirePiRuntime(w http.ResponseWriter, r *http.Request) (PiRuntime, bool) {
	if h.piRuntime == nil {
		h.writeErrorReq(w, r, http.StatusServiceUnavailable, "pi_runtime_unavailable")
		return nil, false
	}
	return h.piRuntime, true
}

func (h *Handlers) writePiRuntimeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusServiceUnavailable
	var runtimeError *PiRuntimeError
	if errors.As(err, &runtimeError) {
		switch runtimeError.StatusCode {
		case http.StatusBadRequest, http.StatusConflict:
			status = http.StatusBadRequest
		case http.StatusNotFound:
			status = http.StatusNotFound
		}
	}
	if errors.Is(err, ErrPiRuntimeUnavailable) {
		status = http.StatusServiceUnavailable
	}
	h.writeErrorReq(w, r, status, "pi_runtime_request_failed")
}

func decodePiJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, piRequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func nonNegativeQueryInteger(r *http.Request, name string) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("query value must be a non-negative integer")
	}
	return parsed, nil
}

func localPiOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() || !piOriginAllowed(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": T(localeFromRequest(r), "pi_local_only")})
			return
		}
		next(w, r)
	}
}

func piOriginAllowed(r *http.Request) bool {
	originValue := strings.TrimSpace(r.Header.Get("Origin"))
	if originValue == "" {
		return true
	}
	origin, err := url.Parse(originValue)
	return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") && origin.Host == r.Host
}
