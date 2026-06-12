package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type SSEMessage struct {
	Event string      `json:"-"`
	Data  interface{} `json:"data"`
}

type LogEntry struct {
	Level   string `json:"level"`
	Msg     string `json:"msg"`
	Time    string `json:"time"`
}

type LogBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan SSEMessage]bool
	closed  bool
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
	entry := LogEntry{
		Level: level,
		Msg:   msg,
		Time:  time.Now().Format("15:04:05"),
	}
	lb.broadcast(SSEMessage{Event: "log", Data: entry})
	fmt.Printf(" [%s] %s\n", level, msg)
}

func (lb *LogBroadcaster) Info(msg string)    { lb.Log("info", msg) }
func (lb *LogBroadcaster) Error(msg string)   { lb.Log("error", msg) }
func (lb *LogBroadcaster) Warn(msg string)    { lb.Log("warn", msg) }
func (lb *LogBroadcaster) Success(msg string) { lb.Log("success", msg) }

func (lb *LogBroadcaster) StepInfo(step, total int, msg string) {
	lb.Log("info", fmt.Sprintf("[%d/%d] %s", step, total, msg))
}

// StreamStart 通知前端：某章节的流式输出即将开始，应清空之前的流式缓冲。
func (lb *LogBroadcaster) StreamStart(chapterIdx int) {
	lb.Emit("stream_start", map[string]interface{}{
		"chapter_idx": chapterIdx,
	})
}

func (lb *LogBroadcaster) StreamProgress(chapterIdx int, charCount int) {
	lb.Emit("stream_progress", map[string]interface{}{
		"chapter_idx": chapterIdx,
		"char_count":  charCount,
	})
}

func (lb *LogBroadcaster) Emit(event string, data interface{}) {
	lb.broadcast(SSEMessage{Event: event, Data: data})
}

func (lb *LogBroadcaster) ProgressUpdate(data interface{}) {
	lb.Emit("progress_update", data)
}

func (lb *LogBroadcaster) TaskStart(task string) {
	lb.Emit("task_start", map[string]string{"task": task})
}

func (lb *LogBroadcaster) TaskEnd(task string, success bool) {
	lb.Emit("task_end", map[string]interface{}{"task": task, "success": success})
}

func (lb *LogBroadcaster) ContentChunk(chapterIdx int, text string) {
	lb.Emit("content_chunk", map[string]interface{}{"chapter_idx": chapterIdx, "text": text})
}

func (lb *LogBroadcaster) ForeshadowSuggestions(suggestions []ForeshadowSuggestion) {
	lb.Emit("foreshadow_suggestions", suggestions)
}

func (lb *LogBroadcaster) ContinueAnalysisResult(data interface{}) {
	lb.Emit("continue_analysis", data)
}

func (lb *LogBroadcaster) SettingsReconciled(data interface{}) {
	lb.Emit("settings_reconciled", data)
}

func (lb *LogBroadcaster) SettingsUpdated() {
	lb.Emit("settings_updated", map[string]string{"status": "ok"})
}

func (lb *LogBroadcaster) PostProcessReport(reportType, content string) {
	lb.Emit("postprocess_report", map[string]string{
		"type":    reportType,
		"content": content,
	})
}

func (lb *LogBroadcaster) PostProcessRoadmap(pp *PostProcessState) {
	lb.Emit("postprocess_roadmap", pp)
}

func (lb *LogBroadcaster) PostProcessItemDone(item RoadmapItem) {
	lb.Emit("postprocess_item_done", item)
}

func (lb *LogBroadcaster) PostProcessUpdate(pp *PostProcessState) {
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

func (lb *LogBroadcaster) ToolCallEnd(sessionID, toolName, result string) {
	lb.Emit("tool_call_end", map[string]interface{}{
		"session_id": sessionID,
		"tool_name":  toolName,
		"result":     result,
	})
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

func formatSSE(msg SSEMessage) []byte {
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
