package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const piRuntimeResponseLimit = 2 * 1024 * 1024

type piRuntimeClient struct {
	baseURL string
	http    *http.Client
}

func NewPiRuntimeClient(socketPath string) PiRuntime {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &piRuntimeClient{
		baseURL: "http://pi-runtime",
		http:    &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}
}

func newPiRuntimeClient(client *http.Client, baseURL string) *piRuntimeClient {
	return &piRuntimeClient{baseURL: strings.TrimRight(baseURL, "/"), http: client}
}

func (c *piRuntimeClient) Health(ctx context.Context) (PiHealth, error) {
	var result PiHealth
	err := c.doJSON(ctx, http.MethodGet, "/health", nil, &result)
	return result, err
}

func (c *piRuntimeClient) Providers(ctx context.Context) ([]PiProvider, error) {
	var result []PiProvider
	err := c.doJSON(ctx, http.MethodGet, "/v1/providers", nil, &result)
	return result, err
}

func (c *piRuntimeClient) Credentials(ctx context.Context) ([]PiCredential, error) {
	var result []PiCredential
	err := c.doJSON(ctx, http.MethodGet, "/v1/credentials", nil, &result)
	return result, err
}

func (c *piRuntimeClient) StartLogin(ctx context.Context, input PiLoginStartRequest) (PiLoginSnapshot, error) {
	var result PiLoginSnapshot
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/logins", input, &result)
	return result, err
}

func (c *piRuntimeClient) LoginStatus(ctx context.Context, id string, after int) (PiLoginSnapshot, error) {
	path := "/v1/auth/logins/" + url.PathEscape(id) + "?after=" + strconv.Itoa(after)
	var result PiLoginSnapshot
	err := c.doJSON(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *piRuntimeClient) RespondLogin(ctx context.Context, id string, input PiLoginResponse) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/logins/"+url.PathEscape(id)+"/responses", input, nil)
}

func (c *piRuntimeClient) CancelLogin(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/auth/logins/"+url.PathEscape(id), nil, nil)
}

func (c *piRuntimeClient) Logout(ctx context.Context, providerID string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/logout", map[string]string{"provider_id": providerID}, nil)
}

func (c *piRuntimeClient) ImportLegacy(ctx context.Context, input PiLegacyImportRequest) (PiLegacyImportResult, error) {
	var result PiLegacyImportResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/legacy-import", input, &result)
	return result, err
}

func (c *piRuntimeClient) SelectProject(ctx context.Context, projectName string) (PiSessionSummary, error) {
	var result PiSessionSummary
	err := c.doJSON(ctx, http.MethodPost, "/v1/sessions/select", map[string]string{"project_name": projectName}, &result)
	return result, err
}

func (c *piRuntimeClient) CurrentSession(ctx context.Context) (*PiSessionSummary, error) {
	var result *PiSessionSummary
	err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/current", nil, &result)
	return result, err
}

func (c *piRuntimeClient) Close() error {
	c.http.CloseIdleConnections()
	return nil
}

func (c *piRuntimeClient) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode pi runtime request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create pi runtime request: %w", err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("pi runtime request failed: %w", err)
	}
	defer response.Body.Close()

	encoded, err := io.ReadAll(io.LimitReader(response.Body, piRuntimeResponseLimit+1))
	if err != nil {
		return fmt.Errorf("read pi runtime response: %w", err)
	}
	if len(encoded) > piRuntimeResponseLimit {
		return fmt.Errorf("pi runtime response exceeds 2 MiB")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodePiRuntimeError(response.StatusCode, encoded)
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(encoded, output); err != nil {
		return fmt.Errorf("decode runtime response: %w", err)
	}
	return nil
}

func decodePiRuntimeError(statusCode int, encoded []byte) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &envelope); err == nil && envelope.Error.Code != "" && envelope.Error.Message != "" {
		return &PiRuntimeError{StatusCode: statusCode, Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	return &PiRuntimeError{
		StatusCode: statusCode,
		Code:       "runtime_error",
		Message:    http.StatusText(statusCode),
	}
}
