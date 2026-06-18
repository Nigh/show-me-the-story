package main

import (
	"context"
	"sync"
	"time"
	"unicode/utf8"
)

type taskTokenCtxKey struct{}

const tokenEmitInterval = 2 * time.Second

// TaskTokenUsage accumulates prompt/completion tokens for one async task.
type TaskTokenUsage struct {
	mu sync.Mutex

	committedPrompt     int
	committedCompletion int
	pendingPrompt       int
	pendingCompletion   int

	logger   *LogBroadcaster
	lastEmit time.Time
}

func newTaskTokenUsage(logger *LogBroadcaster) *TaskTokenUsage {
	return &TaskTokenUsage{logger: logger}
}

func withTaskTokens(ctx context.Context, logger *LogBroadcaster) (context.Context, *TaskTokenUsage) {
	usage := newTaskTokenUsage(logger)
	return context.WithValue(ctx, taskTokenCtxKey{}, usage), usage
}

func taskTokensFromContext(ctx context.Context) *TaskTokenUsage {
	if ctx == nil {
		return nil
	}
	usage, _ := ctx.Value(taskTokenCtxKey{}).(*TaskTokenUsage)
	return usage
}

func countMessageRunes(messages []Message) int {
	n := 0
	for _, m := range messages {
		n += utf8.RuneCountInString(m.Content)
	}
	return n
}

func (t *TaskTokenUsage) beginCall(messages []Message) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.pendingPrompt = estimateTokensFromRunes(countMessageRunes(messages))
	t.pendingCompletion = 0
	t.mu.Unlock()
	t.maybeEmit(true)
}

func (t *TaskTokenUsage) updateStreamContent(content string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.pendingCompletion = estimateTokensFromRunes(utf8.RuneCountInString(content))
	t.mu.Unlock()
	t.maybeEmit(false)
}

func (t *TaskTokenUsage) finishCall(promptTokens, completionTokens int, hasUsage bool, messages []Message, output string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if hasUsage {
		t.committedPrompt += promptTokens
		t.committedCompletion += completionTokens
	} else {
		t.committedPrompt += estimateTokensFromRunes(countMessageRunes(messages))
		t.committedCompletion += estimateTokensFromRunes(utf8.RuneCountInString(output))
	}
	t.pendingPrompt = 0
	t.pendingCompletion = 0
	t.mu.Unlock()
	t.maybeEmit(true)
}

func (t *TaskTokenUsage) Snapshot() (prompt, completion int) {
	if t == nil {
		return 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.committedPrompt + t.pendingPrompt, t.committedCompletion + t.pendingCompletion
}

func (t *TaskTokenUsage) maybeEmit(force bool) {
	if t == nil || t.logger == nil {
		return
	}
	t.mu.Lock()
	now := time.Now()
	if !force && !t.lastEmit.IsZero() && now.Sub(t.lastEmit) < tokenEmitInterval {
		t.mu.Unlock()
		return
	}
	prompt := t.committedPrompt + t.pendingPrompt
	completion := t.committedCompletion + t.pendingCompletion
	t.lastEmit = now
	t.mu.Unlock()
	t.logger.TokenUsage(prompt, completion)
}
