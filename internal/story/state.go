package story

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/fsutil"
)

type ChapterState struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline"`
	// Characters is the structured cast for this chapter's outline (proper names only).
	// Used for unregistered-character suggestions; optional on legacy projects.
	Characters []OutlineChapterCharacter `json:"characters,omitempty"`
	Content    string                    `json:"content,omitempty"`
	Summary    string                    `json:"summary"`
	Status     string                    `json:"status"` // pending | writing | review | accepted
	// WordCount is the prose-unit count of Content, refreshed on save so the
	// frontend can show word counts without fetching full chapter content.
	WordCount int `json:"word_count,omitempty"`
	// ContentRev is a content hash for API responses; the frontend uses it to
	// invalidate its per-chapter content cache. Never persisted.
	ContentRev string `json:"content_rev,omitempty"`
	// Blocks is the editable-block view of Content (one block per paragraph,
	// stable IDs). Persisted in the chapter file, stripped from progress.json
	// and /api/progress; ships with GET /api/chapters/{num}.
	Blocks      []Block `json:"blocks,omitempty"`
	NextBlockID int     `json:"next_block_id,omitempty"`
	BlockSep    string  `json:"block_sep,omitempty"`
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
	TargetChapter int               `json:"target_chapter,omitempty"`
	TargetHorizon string            `json:"target_horizon,omitempty"`
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

type MemoryEntry struct {
	ID       int    `json:"id"`
	Content  string `json:"content"`
	Category string `json:"category"` // character | location | item | event | promise | other
	Chapter  int    `json:"chapter"`
	Position int    `json:"position"`
	// Snippet is resolved server-side for API responses (chapter content no
	// longer travels with /api/progress); never persisted.
	Snippet string `json:"snippet,omitempty"`
}

// Arc is a volume-level unit of the hierarchical outline (v3). Chapter
// outlines are generated arc by arc so books with 1000+ chapters never need
// a single outline call. Status is derived from the chapters in range.
type Arc struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Goal    string `json:"goal"`
	StartCh int    `json:"start_ch"`
	EndCh   int    `json:"end_ch"`
	// Summary is filled by AI once every chapter in range is accepted; it
	// replaces per-chapter context for completed arcs in later prompts.
	Summary string `json:"summary,omitempty"`
}

type Progress struct {
	OutlineBatches              []OutlineBatch           `json:"outline_batches,omitempty"`
	Phase                       string                   `json:"phase"`
	Title                       string                   `json:"title"`
	CorePrompt                  string                   `json:"core_prompt"`
	StorySynopsis               string                   `json:"story_synopsis"`
	Chapters                    []ChapterState           `json:"chapters"`
	Arcs                        []Arc                    `json:"arcs,omitempty"`
	CurrentChapterIndex         int                      `json:"current_chapter_index"`
	StoryConfigSnapshot         *config.StoryConfig      `json:"story_config_snapshot,omitempty"`
	Foreshadows                 []Foreshadow             `json:"foreshadows,omitempty"`
	LastForeshadowOutlineReport *ForeshadowOutlineReport `json:"last_foreshadow_outline_report,omitempty"`
	LastOutlineCharacterReport  *OutlineCharacterReport  `json:"last_outline_character_report,omitempty"`
	PendingWritingConflict      *WritingConflict         `json:"pending_writing_conflict,omitempty"`
	MemoryEntries               []MemoryEntry            `json:"memory_entries,omitempty"`
	MemoryMaxTokens             int                      `json:"memory_max_tokens,omitempty"`
	BookStatus                  string                   `json:"book_status,omitempty"`
	LongTermDirection           string                   `json:"long_term_direction,omitempty"`
	LatestPlanningReview        *PlanningReview          `json:"latest_planning_review,omitempty"`
	NarrativeCheckpoints        []NarrativeCheckpoint    `json:"narrative_checkpoints,omitempty"`
}

const (
	BookStatusActive    = "active"
	BookStatusCompleted = "completed"
)

type PlanningReview struct {
	ThroughChapter int    `json:"through_chapter"`
	Content        string `json:"content"`
	CreatedAt      string `json:"created_at"`
}

type NarrativeCheckpoint struct {
	StartChapter int    `json:"start_chapter"`
	EndChapter   int    `json:"end_chapter"`
	Summary      string `json:"summary"`
}

const (
	StatusPending  = "pending"
	StatusWriting  = "writing"
	StatusReview   = "review"
	StatusAccepted = "accepted"
)

// LoadProgress loads project metadata from path (progress.json) and chapter
// prose from the chapters/ directory next to it (v3 storage layout).
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

	loadChapterContents(path, &p)
	return &p, nil
}

// SaveProgress persists chapter prose to per-chapter files (only chapters
// whose content changed since load/last save are rewritten), then writes the
// metadata file without prose content.
func SaveProgress(path string, p *Progress) error {
	if err := saveChapterFiles(path, p); err != nil {
		return err
	}

	meta := *p
	meta.Chapters = make([]ChapterState, len(p.Chapters))
	for i, ch := range p.Chapters {
		ch.Content = ""
		ch.Blocks = nil
		ch.NextBlockID = 0
		ch.BlockSep = ""
		meta.Chapters[i] = ch
	}
	data, err := json.MarshalIndent(&meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化进度失败: %w", err)
	}
	if err := fsutil.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("保存进度文件失败: %w", err)
	}
	cleanupChapterFiles(path, p)
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
