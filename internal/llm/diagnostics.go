package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"showmethestory/internal/devlog"
)

var requestSequence atomic.Uint64

// safeEndpoint retains routing metadata, never credentials or arbitrary path segments.
func safeEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "invalid"
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, segment := range segments {
		switch segment {
		case "", "api", "openai", "chat", "completions", "models", "coding", "paas":
			continue
		}
		version := len(segment) > 1 && len(segment) <= 4 && segment[0] == 'v'
		for _, ch := range segment[1:] {
			if ch < '0' || ch > '9' {
				version = false
			}
		}
		if !version {
			segments[i] = "[redacted]"
		}
	}
	return u.Scheme + "://" + u.Host + "/" + strings.Join(segments, "/")
}

func registerEndpointSecrets(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	if u.User != nil {
		devlog.RegisterSecret(u.User.Username())
		if password, ok := u.User.Password(); ok {
			devlog.RegisterSecret(password)
		}
	}
	for key, values := range u.Query() {
		key = strings.ToLower(key)
		if !strings.Contains(key, "key") && !strings.Contains(key, "token") &&
			!strings.Contains(key, "secret") && !strings.Contains(key, "password") &&
			!strings.Contains(key, "signature") && !strings.Contains(key, "credential") &&
			!strings.Contains(key, "auth") && key != "sig" {
			continue
		}
		for _, value := range values {
			devlog.RegisterSecret(value)
		}
	}
}

type requestLog struct {
	ctx     context.Context
	id      string
	started time.Time
	attrs   []any
	status  int
	usage   *tokenUsage
}

// Each attempt, including a synchronous stream fallback, gets its own ID.
// Only RecordPayload may write model text; devlog keeps that opt-in.
func beginRequest(ctx context.Context, method, endpoint, model string, stream bool, attrs ...any) *requestLog {
	registerEndpointSecrets(endpoint)
	started := time.Now()
	r := &requestLog{
		ctx:     ctx,
		id:      fmt.Sprintf("llm-%x-%d", started.UnixNano(), requestSequence.Add(1)),
		started: started,
	}
	r.attrs = append([]any{
		"request_id", r.id, "task_id", devlog.TaskID(ctx),
		"method", method, "endpoint", safeEndpoint(endpoint), "model", model, "stream", stream,
	}, attrs...)
	devlog.Event("INFO", "llm_request_start", r.attrs...)
	return r
}

func (r *requestLog) finish(err error, attrs ...any) {
	result, level := "success", "INFO"
	if err != nil {
		result, level = "error", "WARN"
		attrs = append(attrs, "error_kind", safeErrorKind(err), "fatal", IsFatalAPIError(err))
		var status *apiStatusError
		if errors.As(err, &status) {
			if status.code != "" {
				attrs = append(attrs, "error_code", status.code)
			}
			if status.errorType != "" {
				attrs = append(attrs, "error_type", status.errorType)
			}
		}
	}
	attrs = append(attrs, "duration_ms", time.Since(r.started).Milliseconds(), "result", result)
	if r.status != 0 {
		attrs = append(attrs, "status_code", r.status)
	}
	if r.usage != nil {
		attrs = append(attrs, "prompt_tokens", r.usage.PromptTokens, "completion_tokens", r.usage.CompletionTokens, "total_tokens", r.usage.TotalTokens)
	}
	devlog.Event(level, "llm_request_end", append(r.attrs, attrs...)...)
}

func (r *requestLog) complete(result CompletionResult, err error) {
	attrs := []any{"output_bytes", len(result.Content)}
	if result.FinishReason != "" {
		attrs = append(attrs, "finish_reason", safeFinishReason(result.FinishReason))
	}
	r.finish(err, attrs...)
	if result.Content != "" {
		devlog.RecordPayload(r.ctx, r.id, "model_output", result.Content)
	}
}

func safeFinishReason(reason string) string {
	switch reason {
	case "stop", "length", "tool_calls", "function_call", "content_filter":
		return reason
	default:
		return "other"
	}
}

// Keep the cause for cancellation and retry classification, not for display.
type safeRequestError struct {
	message string
	cause   error
}

func (e *safeRequestError) Error() string { return e.message }
func (e *safeRequestError) Unwrap() error { return e.cause }

func safeErrorKind(err error) string {
	var status *apiStatusError
	var dns *net.DNSError
	var syntax *json.SyntaxError
	var fieldType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &status):
		return "http_status"
	case errors.Is(err, context.Canceled):
		return "context canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.As(err, &dns) && dns.IsNotFound:
		return "no such host"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected EOF"
	case errors.As(err, &syntax), errors.As(err, &fieldType):
		return "invalid_json"
	default:
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return "timeout"
		}
		return "request failed"
	}
}

func safeFailure(stage string, err error) error {
	var safe *safeRequestError
	if errors.As(err, &safe) {
		return &safeRequestError{message: stage + ": " + safe.Error(), cause: err}
	}
	return &safeRequestError{message: stage + ": " + safeErrorKind(err), cause: err}
}

// Status controls retry semantics; only known provider labels may be displayed.
type apiStatusError struct {
	status    int
	code      string
	errorType string
}

func readAPIStatusError(status int, body io.Reader) *apiStatusError {
	result := &apiStatusError{status: status}
	const maxBytes = 64 << 10
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return result
	}
	var envelope struct {
		Error struct {
			Code json.RawMessage `json:"code"`
			Type json.RawMessage `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil {
		result.code = safeProviderErrorLabel(envelope.Error.Code)
		result.errorType = safeProviderErrorLabel(envelope.Error.Type)
	}
	return result
}

func safeProviderErrorLabel(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	switch value {
	case "invalid_api_key", "context_length_exceeded", "insufficient_quota",
		"rate_limit_exceeded", "invalid_request_error", "authentication_error",
		"permission_error", "not_found_error", "server_error", "rate_limit_error",
		"api_error", "overloaded_error", "bad_request_error", "model_not_found":
		return value
	default:
		return ""
	}
}

func (e *apiStatusError) Error() string {
	message := fmt.Sprintf("API 响应错误，状态码: %d", e.status)
	if e.code != "" {
		message += ", code: " + e.code
	}
	if e.errorType != "" {
		message += ", type: " + e.errorType
	}
	return message
}
