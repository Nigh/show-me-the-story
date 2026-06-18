package main

import (
	"fmt"
	"strings"
)

// EditOp represents the type of surgical edit operation.
type EditOp string

const (
	EditOpReplaceLines    EditOp = "replace_lines"     // Replace a range of lines
	EditOpReplaceText     EditOp = "replace_text"      // Find and replace a text snippet
	EditOpInsertAfterLine EditOp = "insert_after_line" // Insert content after a line
	EditOpAppend          EditOp = "append"            // Append content at the end
)

// EditChapterContentRequest holds parameters for a surgical chapter edit.
type EditChapterContentRequest struct {
	ChapterNum int    `json:"num"`
	Operation  EditOp `json:"operation"`
	StartLine  int    `json:"start_line,omitempty"` // 1-indexed, inclusive (replace_lines)
	EndLine    int    `json:"end_line,omitempty"`   // 1-indexed, inclusive (replace_lines)
	OldText    string `json:"old_text,omitempty"`   // exact text to find (replace_text)
	Line       int    `json:"line,omitempty"`       // line number (insert_after_line)
	NewText    string `json:"new_text"`             // replacement/insertion content
}

// EditChapterContent performs a surgical edit on a chapter's content.
// Returns the number of lines in the resulting content and any error.
func EditChapterContent(state *Progress, req EditChapterContentRequest) (int, error) {
	// Find the chapter
	var ch *ChapterState
	for i := range state.Chapters {
		if state.Chapters[i].Num == req.ChapterNum {
			ch = &state.Chapters[i]
			break
		}
	}
	if ch == nil {
		return 0, fmt.Errorf("第 %d 章不存在", req.ChapterNum)
	}
	if ch.Content == "" {
		return 0, fmt.Errorf("第 %d 章正文为空，无法编辑", req.ChapterNum)
	}

	lines := strings.Split(ch.Content, "\n")
	totalLines := len(lines)

	switch req.Operation {
	case EditOpReplaceLines:
		if req.StartLine < 1 || req.StartLine > totalLines {
			return 0, fmt.Errorf("起始行 %d 超出范围（共 %d 行）", req.StartLine, totalLines)
		}
		if req.EndLine < req.StartLine || req.EndLine > totalLines {
			return 0, fmt.Errorf("结束行 %d 超出范围（起始行 %d，共 %d 行）", req.EndLine, req.StartLine, totalLines)
		}
		newLines := strings.Split(req.NewText, "\n")
		result := make([]string, 0, len(lines)-(req.EndLine-req.StartLine+1)+len(newLines))
		result = append(result, lines[:req.StartLine-1]...)
		result = append(result, newLines...)
		result = append(result, lines[req.EndLine:]...)
		ch.Content = strings.Join(result, "\n")

	case EditOpReplaceText:
		if req.OldText == "" {
			return 0, fmt.Errorf("old_text 不能为空")
		}
		idx := strings.Index(ch.Content, req.OldText)
		if idx < 0 {
			return 0, fmt.Errorf("未找到匹配文本（长度 %d 字符）", len(req.OldText))
		}
		ch.Content = ch.Content[:idx] + req.NewText + ch.Content[idx+len(req.OldText):]

	case EditOpInsertAfterLine:
		if req.Line < 0 || req.Line > totalLines {
			return 0, fmt.Errorf("行号 %d 超出范围（共 %d 行，0 表示文件开头）", req.Line, totalLines)
		}
		newLines := strings.Split(req.NewText, "\n")
		result := make([]string, 0, len(lines)+len(newLines))
		result = append(result, lines[:req.Line]...)
		result = append(result, newLines...)
		result = append(result, lines[req.Line:]...)
		ch.Content = strings.Join(result, "\n")

	case EditOpAppend:
		if ch.Content != "" && !strings.HasSuffix(ch.Content, "\n") {
			ch.Content += "\n"
		}
		ch.Content += req.NewText

	default:
		return 0, fmt.Errorf("未知编辑操作: %s", req.Operation)
	}

	return len(strings.Split(ch.Content, "\n")), nil
}

// findChapterIdx returns the index of the chapter with the given num, or -1.
func findChapterIdx(state *Progress, num int) int {
	for i, ch := range state.Chapters {
		if ch.Num == num {
			return i
		}
	}
	return -1
}
