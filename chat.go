package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ChatSession struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Messages  []ChatMessage `json:"messages"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
}

type ChatMessage struct {
	Role            string     `json:"role"`
	Content         string     `json:"content"`
	ToolCalls       []ToolCall `json:"tool_calls,omitempty"`
	ToolResult      string     `json:"tool_result,omitempty"`
	ToolResultKey   string     `json:"tool_result_key,omitempty"`
	ToolResultArgs  []string   `json:"tool_result_args,omitempty"`
	Timestamp       string     `json:"timestamp"`
}

type ChatSessionIndex struct {
	Sessions []ChatSessionMeta `json:"sessions"`
}

type ChatSessionMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	MsgCount  int    `json:"msg_count"`
}

func chatSessionsDir(baseDir string) string {
	return filepath.Join(baseDir, "sessions")
}

func chatIndexPath(baseDir string) string {
	return filepath.Join(chatSessionsDir(baseDir), "index.json")
}

func isValidSessionID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if c == '/' || c == '\\' || c == '.' || c == ':' {
			return false
		}
	}
	return true
}

func LoadChatSessions(baseDir string) (*ChatSessionIndex, error) {
indexPath := chatIndexPath(baseDir)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ChatSessionIndex{}, nil
		}
		return nil, fmt.Errorf("读取会话索引失败: %w", err)
	}

	var idx ChatSessionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析会话索引失败: %w", err)
	}

	return &idx, nil
}

func saveChatSessions(baseDir string, idx *ChatSessionIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(chatIndexPath(baseDir), data)
}

func LoadChatSession(baseDir, id string) (*ChatSession, error) {
	if !isValidSessionID(id) {
		return nil, fmt.Errorf("无效的会话ID")
	}
	path := filepath.Join(chatSessionsDir(baseDir), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("会话不存在: %s", id)
		}
		return nil, fmt.Errorf("读取会话失败: %w", err)
	}

	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("解析会话失败: %w", err)
	}

	return &session, nil
}

func SaveChatSession(baseDir string, session *ChatSession) error {
	dir := chatSessionsDir(baseDir)
	os.MkdirAll(dir, 0755)

	path := filepath.Join(dir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}

	return updateChatIndex(baseDir, session)
}

func DeleteChatSession(baseDir, id string) error {
	if !isValidSessionID(id) {
		return fmt.Errorf("无效的会话ID")
	}
	path := filepath.Join(chatSessionsDir(baseDir), id+".json")
	if err := deleteFile(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	idx, err := LoadChatSessions(baseDir)
	if err != nil {
		return err
	}

	var filtered []ChatSessionMeta
	for _, m := range idx.Sessions {
		if m.ID != id {
			filtered = append(filtered, m)
		}
	}
	idx.Sessions = filtered

	return saveChatSessions(baseDir, idx)
}

func updateChatIndex(baseDir string, session *ChatSession) error {
	idx, err := LoadChatSessions(baseDir)
	if err != nil {
		return err
	}

	found := false
	for i, m := range idx.Sessions {
		if m.ID == session.ID {
			idx.Sessions[i].Title = session.Title
			idx.Sessions[i].UpdatedAt = session.UpdatedAt
			idx.Sessions[i].MsgCount = len(session.Messages)
			found = true
			break
		}
	}

	if !found {
		idx.Sessions = append(idx.Sessions, ChatSessionMeta{
			ID:        session.ID,
			Title:     session.Title,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
			MsgCount:  len(session.Messages),
		})
	}

	return saveChatSessions(baseDir, idx)
}

func generateSessionID() string {
	return fmt.Sprintf("s_%d", time.Now().UnixNano())
}

func generateChatTitle(userMessage string) string {
	runes := []rune(userMessage)
	if len(runes) > 20 {
		return string(runes[:20]) + "..."
	}
	return userMessage
}
