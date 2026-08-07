package sse

import (
	"encoding/json"
	"fmt"
	"showmethestory/internal/devlog"
	"showmethestory/internal/i18n"
	"sync"
	"time"
)

type SSEMessage struct {
	Event string      `json:"-"`
	Data  interface{} `json:"data"`
}

type LogEntry struct {
	Level   string   `json:"level"`
	Msg     string   `json:"msg"`               // Chinese text (default fallback)
	MsgEN   string   `json:"msg_en,omitempty"`  // English text fallback
	MsgKey  string   `json:"msg_key,omitempty"` // stable i18n key for frontend
	MsgArgs []string `json:"msg_args,omitempty"`
	Time    string   `json:"time"`
}

type LogBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan SSEMessage]bool
	closed  bool

	taskMu sync.Mutex
	tasks  []string // nested task names for GetStatus / UI hydrate after refresh
}

func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		clients: make(map[chan SSEMessage]bool),
	}
}

func (lb *LogBroadcaster) Subscribe() chan SSEMessage {
	ch := make(chan SSEMessage, 64)
	lb.mu.Lock()
	lb.clients[ch] = true
	lb.mu.Unlock()
	return ch
}

func (lb *LogBroadcaster) Unsubscribe(ch chan SSEMessage) {
	lb.mu.Lock()
	delete(lb.clients, ch)
	lb.mu.Unlock()
	close(ch)
}

func (lb *LogBroadcaster) broadcast(msg SSEMessage) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if lb.closed {
		return
	}
	for ch := range lb.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (lb *LogBroadcaster) Log(level, msg string) {
	lb.logEntry(LogEntry{
		Level: level,
		Msg:   msg,
		Time:  time.Now().Format("15:04:05"),
	})
}

func (lb *LogBroadcaster) logBilingual(level, msg, msgEN string) {
	lb.logEntry(LogEntry{
		Level: level,
		Msg:   msg,
		MsgEN: msgEN,
		Time:  time.Now().Format("15:04:05"),
	})
}

func (lb *LogBroadcaster) logKey(level, key string, args ...any) {
	lb.logEntry(LogEntry{
		Level:   level,
		MsgKey:  key,
		MsgArgs: i18n.MsgArgs(args...),
		Msg:     i18n.T(i18n.LangZH, key, args...),
		MsgEN:   i18n.T(i18n.LangEN, key, args...),
		Time:    time.Now().Format("15:04:05"),
	})
}

func (lb *LogBroadcaster) logEntry(entry LogEntry) {
	lb.broadcast(SSEMessage{Event: "log", Data: entry})
	if entry.Msg != "" {
		fmt.Printf(" [%s] %s\n", entry.Level, entry.Msg)
	} else if entry.MsgKey != "" {
		fmt.Printf(" [%s] %s\n", entry.Level, entry.MsgKey)
	}
	// ponytail: file log only warn/error (+ keyed agent failures); skip info/step spam.
	if entry.Level == "warn" || entry.Level == "error" {
		msg := entry.Msg
		if msg == "" {
			msg = entry.MsgKey
		}
		devlog.Log("%s %s", entry.Level, msg)
	}
}

func (lb *LogBroadcaster) Info(msg string)    { lb.Log("info", msg) }
func (lb *LogBroadcaster) Error(msg string)   { lb.Log("error", msg) }
func (lb *LogBroadcaster) Warn(msg string)    { lb.Log("warn", msg) }
func (lb *LogBroadcaster) Success(msg string) { lb.Log("success", msg) }

func (lb *LogBroadcaster) InfoKey(key string, args ...any)    { lb.logKey("info", key, args...) }
func (lb *LogBroadcaster) ErrorKey(key string, args ...any)   { lb.logKey("error", key, args...) }
func (lb *LogBroadcaster) WarnKey(key string, args ...any)    { lb.logKey("warn", key, args...) }
func (lb *LogBroadcaster) SuccessKey(key string, args ...any) { lb.logKey("success", key, args...) }

// *Bilingual variants supply both zh and en text; frontend picks based on UI locale.
func (lb *LogBroadcaster) InfoBilingual(zh, en string)    { lb.logBilingual("info", zh, en) }
func (lb *LogBroadcaster) ErrorBilingual(zh, en string)   { lb.logBilingual("error", zh, en) }
func (lb *LogBroadcaster) WarnBilingual(zh, en string)    { lb.logBilingual("warn", zh, en) }
func (lb *LogBroadcaster) SuccessBilingual(zh, en string) { lb.logBilingual("success", zh, en) }

func (lb *LogBroadcaster) StepInfo(step, total int, msg string) {
	lb.Log("info", fmt.Sprintf("[%d/%d] %s", step, total, msg))
}
func (lb *LogBroadcaster) StreamStart(chapterIdx int) {
	lb.Emit("stream_start", map[string]interface{}{
		"chapter_idx": chapterIdx,
	})
}

func (lb *LogBroadcaster) TokenUsage(promptTokens, completionTokens int) {
	lb.Emit("token_usage", map[string]int{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
	})
}

func (lb *LogBroadcaster) Emit(event string, data interface{}) {
	lb.broadcast(SSEMessage{Event: event, Data: data})
}

func (lb *LogBroadcaster) ProgressUpdate(data interface{}) {
	lb.Emit("progress_update", data)
}

func (lb *LogBroadcaster) TaskStart(task string) {
	lb.taskMu.Lock()
	lb.tasks = append(lb.tasks, task)
	lb.taskMu.Unlock()
	devlog.Log("task_start %s", task)
	lb.Emit("task_start", map[string]string{"task": task})
}

func (lb *LogBroadcaster) TaskEnd(task string, success bool) {
	lb.taskMu.Lock()
	for i := len(lb.tasks) - 1; i >= 0; i-- {
		if lb.tasks[i] == task {
			lb.tasks = append(lb.tasks[:i], lb.tasks[i+1:]...)
			break
		}
	}
	lb.taskMu.Unlock()
	devlog.Log("task_end %s success=%v", task, success)
	lb.Emit("task_end", map[string]interface{}{"task": task, "success": success})
}

// CurrentTask returns the innermost running task name, or "".
func (lb *LogBroadcaster) CurrentTask() string {
	lb.taskMu.Lock()
	defer lb.taskMu.Unlock()
	if len(lb.tasks) == 0 {
		return ""
	}
	return lb.tasks[len(lb.tasks)-1]
}

func (lb *LogBroadcaster) ContentChunk(chapterIdx int, text string) {
	lb.Emit("content_chunk", map[string]interface{}{"chapter_idx": chapterIdx, "text": text})
}

func (lb *LogBroadcaster) ForeshadowSuggestions(suggestions any) {
	lb.Emit("foreshadow_suggestions", suggestions)
}

func (lb *LogBroadcaster) ForeshadowOutlineConflicts(report any) {
	lb.Emit("foreshadow_outline_conflicts", report)
}

func (lb *LogBroadcaster) OutlineCharacterSuggestions(suggestions any) {
	lb.Emit("outline_character_suggestions", suggestions)
}

func (lb *LogBroadcaster) WritingConflict(conflict any) {
	lb.Emit("writing_conflict", conflict)
}

func (lb *LogBroadcaster) SettingsReconciled(data interface{}) {
	lb.Emit("settings_reconciled", data)
}

func (lb *LogBroadcaster) SettingsUpdated() {
	lb.Emit("settings_updated", map[string]string{"status": "ok"})
}

func (lb *LogBroadcaster) ConfigChangeProposal(changes any) {
	lb.Emit("config_change_proposal", changes)
}

func (lb *LogBroadcaster) PostProcessReport(reportType, content string) {
	lb.Emit("postprocess_report", map[string]string{
		"type":    reportType,
		"content": content,
	})
}

func (lb *LogBroadcaster) PostProcessRoadmap(pp any) {
	lb.Emit("postprocess_roadmap", pp)
}

func (lb *LogBroadcaster) PostProcessItemDone(item any) {
	lb.Emit("postprocess_item_done", item)
}

func (lb *LogBroadcaster) PostProcessUpdate(pp any) {
	lb.Emit("postprocess_update", pp)
}

func (lb *LogBroadcaster) PolishResult(chapterIdx int, text string) {
	lb.Emit("polish_result", map[string]interface{}{
		"chapter_idx": chapterIdx,
		"text":        text,
	})
}

func (lb *LogBroadcaster) ChatChunk(sessionID, text string) {
	lb.Emit("chat_chunk", map[string]interface{}{
		"session_id": sessionID,
		"text":       text,
	})
}

func (lb *LogBroadcaster) ToolCallStart(sessionID, toolName, args string) {
	lb.Emit("tool_call_start", map[string]interface{}{
		"session_id": sessionID,
		"tool_name":  toolName,
		"args":       args,
	})
}

func (lb *LogBroadcaster) ToolCallEnd(sessionID, toolName, result, resultKey string, resultArgs []string) {
	payload := map[string]interface{}{
		"session_id": sessionID,
		"tool_name":  toolName,
		"result":     result,
	}
	if resultKey != "" {
		payload["result_key"] = resultKey
		payload["result_args"] = resultArgs
	}
	lb.Emit("tool_call_end", payload)
}

func (lb *LogBroadcaster) Close() {
	lb.mu.Lock()
	lb.closed = true
	for ch := range lb.clients {
		close(ch)
	}
	lb.clients = make(map[chan SSEMessage]bool)
	lb.mu.Unlock()
}

func Format(msg SSEMessage) []byte {
	event := msg.Event
	if event == "" {
		event = "message"
	}
	dataBytes, err := json.Marshal(msg.Data)
	if err != nil {
		dataBytes = []byte(`{"error":"marshal failed"}`)
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(dataBytes)))
}
