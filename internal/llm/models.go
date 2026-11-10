package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"showmethestory/internal/config"
	"showmethestory/internal/devlog"
)

type ModelInfo struct {
	ID            string `json:"id"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// FetchModels uses the same base URL and credentials as chat; model metadata is optional.
func FetchModels(ctx context.Context, cfg *config.APIConfig) (models []ModelInfo, err error) {
	if cfg == nil || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("API base URL is required")
	}
	endpoint := resolveAPIBase(cfg.BaseURL, cfg.URLStrict) + "/models"
	devlog.RegisterSecret(cfg.APIKey)
	diagnostic := beginRequest(ctx, "GET", endpoint, cfg.Model, false, "operation", "model_list")
	defer func() { diagnostic.finish(err) }()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, safeFailure("invalid model endpoint", err)
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, safeFailure("model list request", err)
	}
	defer resp.Body.Close()
	diagnostic.status = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		return nil, readAPIStatusError(resp.StatusCode, resp.Body)
	}
	const maxBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, safeFailure("read model list", err)
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("model list exceeds 4 MiB")
	}
	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextWindow int    `json:"context_window"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, safeFailure("invalid model list JSON", err)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("model list is missing data array")
	}
	models = make([]ModelInfo, 0, len(result.Data))
	seen := map[string]bool{}
	for _, item := range result.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		window := item.ContextWindow
		if window <= 0 {
			window = max(0, item.ContextLength)
		}
		models = append(models, ModelInfo{ID: id, ContextWindow: window})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
