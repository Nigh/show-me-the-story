package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func normalizeURL(base string) string {
	base = strings.TrimSpace(base)
	base = strings.TrimSuffix(base, "/")

	if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		if !strings.Contains(base, "11434") {
			base = base + "/v1"
		}
	}

	return base + "/chat/completions"
}

func validateAPIConfig(apiCfg *APIConfig) error {
	if strings.TrimSpace(apiCfg.BaseURL) == "" {
		return fmt.Errorf("API Base URL 未配置")
	}
	if strings.TrimSpace(apiCfg.Model) == "" {
		return fmt.Errorf("Model 未配置")
	}
	return nil
}

func isFatalAPIError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 注意：不要把所有 "dial tcp" 都当作致命错误——
	// "dial tcp ... i/o timeout" 等临时网络故障应当重试。
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") {
		return true
	}
	if strings.Contains(msg, "状态码: 401") ||
		strings.Contains(msg, "状态码: 403") ||
		strings.Contains(msg, "状态码: 404") {
		return true
	}
	if strings.Contains(msg, "context canceled") {
		return true
	}
	return false
}

func CallAPI(ctx context.Context, apiCfg *APIConfig, system, user string) (string, error) {
	return CallAPIMessages(ctx, apiCfg, []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// CallAPIMessages 以完整的多轮消息数组调用 API（非流式）。
func CallAPIMessages(ctx context.Context, apiCfg *APIConfig, messages []Message) (string, error) {
	fullURL := normalizeURL(apiCfg.BaseURL)

	reqBody := ChatRequest{
		Model:    apiCfg.Model,
		Messages: messages,
	}

	bts, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewBuffer(bts))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiCfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiCfg.APIKey)
	}

	timeout := time.Duration(apiCfg.HTTPTimeoutSeconds) * time.Second
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API 响应错误，状态码: %d, 返回内容: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("接口未响应有效 Choices 文本")
}

func CallAPIWithRetry(ctx context.Context, apiCfg *APIConfig, system, user string) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := CallAPI(ctx, apiCfg, system, user)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			fmt.Printf(" ❌ [致命错误] %v，不再重试\n", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] API调用失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func CallAPIWithRetryLog(ctx context.Context, apiCfg *APIConfig, system, user string, logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := CallAPI(ctx, apiCfg, system, user)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			logger.Error(fmt.Sprintf("致命错误: %v，不再重试", err))
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.Warn(fmt.Sprintf("API调用失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime))
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func getWaitTime(retry int) int {
	if retry > 6 {
		return 30
	}
	return retry * 5
}

type streamDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func CallAPIStream(ctx context.Context, apiCfg *APIConfig, system, user string, onChunk func(string)) (string, error) {
	return CallAPIStreamMessages(ctx, apiCfg, []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, onChunk)
}

// CallAPIStreamMessages 以完整的多轮消息数组调用 API（流式）。
func CallAPIStreamMessages(ctx context.Context, apiCfg *APIConfig, messages []Message, onChunk func(string)) (string, error) {
	fullURL := normalizeURL(apiCfg.BaseURL)

	reqBody := ChatRequest{
		Model:    apiCfg.Model,
		Messages: messages,
		Stream:   true,
	}

	bts, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewBuffer(bts))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if apiCfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiCfg.APIKey)
	}

	timeout := time.Duration(apiCfg.HTTPTimeoutSeconds) * time.Second
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API 响应错误，状态码: %d, 返回内容: %s", resp.StatusCode, string(bodyBytes))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return fullContent.String(), ctx.Err()
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var delta streamDelta
		if err := json.Unmarshal([]byte(data), &delta); err != nil {
			continue
		}
		if len(delta.Choices) > 0 && delta.Choices[0].Delta.Content != "" {
			chunk := delta.Choices[0].Delta.Content
			fullContent.WriteString(chunk)
			if onChunk != nil {
				onChunk(chunk)
			}
		}
	}

	result := fullContent.String()
	if result == "" {
		return "", fmt.Errorf("流式响应为空")
	}
	return result, nil
}

func CallAPIStreamWithRetry(ctx context.Context, apiCfg *APIConfig, system, user string, onChunk func(string)) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := CallAPIStream(ctx, apiCfg, system, user, onChunk)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			fmt.Printf(" ❌ [致命错误] %v，不再重试\n", err)
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		fmt.Printf(" ⚠️ [错误] 流式API调用失败: %v。第 %d 次重试，等待 %ds 后重试...\n", err, retryCount, waitTime)
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}

func CallAPIStreamWithRetryLog(ctx context.Context, apiCfg *APIConfig, system, user string, onChunk func(string), logger *LogBroadcaster) string {
	retryCount := 0
	for {
		if ctx.Err() != nil {
			return ""
		}
		result, err := CallAPIStream(ctx, apiCfg, system, user, onChunk)
		if err == nil && result != "" {
			return result
		}
		if isFatalAPIError(err) {
			logger.Error(fmt.Sprintf("致命错误: %v，不再重试", err))
			return ""
		}

		retryCount++
		waitTime := getWaitTime(retryCount)
		logger.Warn(fmt.Sprintf("流式API调用失败: %v。第 %d 次重试，等待 %ds...", err, retryCount, waitTime))
		select {
		case <-time.After(time.Duration(waitTime) * time.Second):
		case <-ctx.Done():
			return ""
		}
	}
}
