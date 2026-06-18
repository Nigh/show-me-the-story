package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ChapterState struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline"`
	Content string `json:"content"`
	Summary string `json:"summary"`
	Status  string `json:"status"` // pending | writing | review | accepted
}

type ForeshadowStatus string

const (
	ForeshadowPlanted     ForeshadowStatus = "planted"
	ForeshadowProgressing ForeshadowStatus = "progressing"
	ForeshadowResolved    ForeshadowStatus = "resolved"
	ForeshadowAbandoned   ForeshadowStatus = "abandoned"
)

type ForeshadowEvent struct {
	Chapter int    `json:"chapter"`
	Note    string `json:"note"`
}

type Foreshadow struct {
	ID            int               `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	PlantChapter  int               `json:"plant_chapter"`
	TargetChapter int               `json:"target_chapter"`
	Status        ForeshadowStatus  `json:"status"`
	Events        []ForeshadowEvent `json:"events"`
	Resolution    string            `json:"resolution"`
}

type ForeshadowOutlineConflict struct {
	ForeshadowID   int    `json:"foreshadow_id"`
	ForeshadowName string `json:"foreshadow_name"`
	ConflictType   string `json:"conflict_type"`
	Description    string `json:"description"`
	SuggestedFix   string `json:"suggested_fix"`
}

type ForeshadowOutlineReport struct {
	HasConflicts bool                        `json:"has_conflicts"`
	Conflicts    []ForeshadowOutlineConflict `json:"conflicts"`
	Summary      string                      `json:"summary"`
}

type ConflictActionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WritingConflict struct {
	ChapterIndex     int                    `json:"chapter_index"`
	ChapterNum       int                    `json:"chapter_num"`
	ChapterTitle     string                 `json:"chapter_title"`
	Issues           []string               `json:"issues"`
	Summary          string                 `json:"summary"`
	RootCause        string                 `json:"root_cause"`
	Reconcilable     bool                   `json:"reconcilable"`
	SuggestedActions []ConflictActionOption `json:"suggested_actions"`
}

type Progress struct {
	Phase                       string                   `json:"phase"`
	Title                       string                   `json:"title"`
	CorePrompt                  string                   `json:"core_prompt"`
	StorySynopsis               string                   `json:"story_synopsis"`
	Chapters                    []ChapterState           `json:"chapters"`
	CurrentChapterIndex         int                      `json:"current_chapter_index"`
	StoryConfigSnapshot         *StoryConfig             `json:"story_config_snapshot,omitempty"`
	Foreshadows                 []Foreshadow             `json:"foreshadows,omitempty"`
	LastForeshadowOutlineReport *ForeshadowOutlineReport `json:"last_foreshadow_outline_report,omitempty"`
	PendingWritingConflict      *WritingConflict         `json:"pending_writing_conflict,omitempty"`
}

const (
	StatusPending  = "pending"
	StatusWriting  = "writing"
	StatusReview   = "review"
	StatusAccepted = "accepted"
)

func LoadProgress(path string) (*Progress, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取进度文件失败: %w", err)
	}

	var p Progress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("解析进度文件失败: %w", err)
	}

	return &p, nil
}

func SaveProgress(path string, p *Progress) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化进度失败: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("保存进度文件失败: %w", err)
	}
	return nil
}

// ChapterMarkdownPath returns the markdown file path for a chapter inside the project directory.
func ChapterMarkdownPath(projectDir string, num int) string {
	return filepath.Join(projectDir, fmt.Sprintf("Chapter_%02d.md", num))
}

func SaveChapterMarkdown(projectDir string, ch ChapterState, title string) {
	content := fmt.Sprintf("# 第 %d 章: %s\n\n> **本章摘要**：%s\n\n---\n\n%s", ch.Num, ch.Title, ch.Summary, ch.Content)
	_ = os.WriteFile(ChapterMarkdownPath(projectDir, ch.Num), []byte(content), 0644)
}

// ForeshadowRoadmapPath returns the foreshadow roadmap markdown path inside the project directory.
func ForeshadowRoadmapPath(projectDir string) string {
	return filepath.Join(projectDir, "Foreshadows.md")
}
