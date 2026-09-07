package story

// v3 block model: chapter prose is exposed as an ordered list of editable
// blocks (one per natural paragraph). In-memory ch.Content stays the single
// source of truth for all AI flows; blocks are (re)derived from content at
// save time with stable IDs, and block edits rebuild content from blocks.

import (
	"context"
	"fmt"
	"path/filepath"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/llm"
	"showmethestory/internal/sse"
	"strings"
)

const (
	BlockParagraph  = "paragraph"
	BlockDialogue   = "dialogue"
	BlockSceneBreak = "scene_break"
)

type Block struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

// splitBlockParagraphs splits content the same way segment revision does:
// blank-line paragraphs first, single-newline fallback. Returns paragraphs
// and the separator to use when rebuilding.
func splitBlockParagraphs(content string) ([]string, string) {
	sep := "\n\n"
	paras := strings.Split(content, sep)
	if len(paras) <= 1 && strings.Contains(content, "\n") {
		sep = "\n"
		paras = strings.Split(content, sep)
	}
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	return out, sep
}

// classifyBlock is a display hint only; misclassification is harmless.
func classifyBlock(text string) string {
	t := strings.TrimSpace(text)
	switch t {
	case "---", "----", "＊＊＊", "***", "⁂", "* * *":
		return BlockSceneBreak
	}
	switch {
	case strings.HasPrefix(t, "“"), strings.HasPrefix(t, "「"), strings.HasPrefix(t, "『"),
		strings.HasPrefix(t, "\""), strings.HasPrefix(t, "'"), strings.HasPrefix(t, "‘"):
		return BlockDialogue
	}
	return BlockParagraph
}

// SyncChapterBlocks re-derives ch.Blocks from ch.Content, preserving the IDs
// of blocks whose text is unchanged (first-unused exact match, in order).
func SyncChapterBlocks(ch *ChapterState) {
	paras, sep := splitBlockParagraphs(ch.Content)
	ch.BlockSep = sep

	used := make([]bool, len(ch.Blocks))
	next := ch.NextBlockID
	if next <= 0 {
		next = 1
	}
	blocks := make([]Block, 0, len(paras))
	for _, p := range paras {
		id := 0
		for i, old := range ch.Blocks {
			if !used[i] && old.Text == p {
				used[i] = true
				id = old.ID
				break
			}
		}
		if id == 0 {
			id = next
			next++
		}
		blocks = append(blocks, Block{ID: id, Type: classifyBlock(p), Text: p})
	}
	ch.Blocks = blocks
	ch.NextBlockID = next
}

// rebuildContentFromBlocks makes ch.Content match ch.Blocks after a block edit.
func rebuildContentFromBlocks(ch *ChapterState) {
	sep := ch.BlockSep
	if sep == "" {
		sep = "\n\n"
	}
	texts := make([]string, len(ch.Blocks))
	for i, b := range ch.Blocks {
		texts[i] = b.Text
	}
	ch.Content = strings.Join(texts, sep)
}

func FindBlockIdx(ch *ChapterState, blockID int) int {
	for i, b := range ch.Blocks {
		if b.ID == blockID {
			return i
		}
	}
	return -1
}

var ErrBlockNotFound = fmt.Errorf("block not found")

// UpdateBlock replaces the text of one block and rebuilds chapter content.
func UpdateBlock(ch *ChapterState, blockID int, text string) error {
	i := FindBlockIdx(ch, blockID)
	if i < 0 {
		return ErrBlockNotFound
	}
	ch.Blocks[i].Text = text
	ch.Blocks[i].Type = classifyBlock(text)
	rebuildContentFromBlocks(ch)
	SyncChapterBlocks(ch)
	return nil
}

// DeleteBlock removes one block and rebuilds chapter content.
func DeleteBlock(ch *ChapterState, blockID int) error {
	i := FindBlockIdx(ch, blockID)
	if i < 0 {
		return ErrBlockNotFound
	}
	ch.Blocks = append(ch.Blocks[:i], ch.Blocks[i+1:]...)
	rebuildContentFromBlocks(ch)
	SyncChapterBlocks(ch)
	return nil
}

// InsertBlockAfter inserts a new block after blockID (0 = at the beginning)
// and returns the new block's ID.
func InsertBlockAfter(ch *ChapterState, blockID int, text string) (int, error) {
	pos := 0
	if blockID != 0 {
		i := FindBlockIdx(ch, blockID)
		if i < 0 {
			return 0, ErrBlockNotFound
		}
		pos = i + 1
	}
	if ch.NextBlockID <= 0 {
		ch.NextBlockID = 1
	}
	nb := Block{ID: ch.NextBlockID, Type: classifyBlock(text), Text: text}
	ch.NextBlockID++
	ch.Blocks = append(ch.Blocks[:pos], append([]Block{nb}, ch.Blocks[pos:]...)...)
	rebuildContentFromBlocks(ch)
	SyncChapterBlocks(ch)
	return nb.ID, nil
}

// ReviseBlockAction runs an AI revision scoped to a single block, using the
// ChapterSegmentRevision prompt. The AI reply replaces the block; if it
// contains multiple paragraphs the block is split on the next sync.
func ReviseBlockAction(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, progressPath string, chapterNum, blockID int, feedback string, settings *ProjectSettings, logger *sse.LogBroadcaster) error {
	chapterIdx := FindChapterIdx(state, chapterNum)
	if chapterIdx < 0 {
		return fmt.Errorf("第 %d 章不存在", chapterNum)
	}
	ch := &state.Chapters[chapterIdx]
	bi := FindBlockIdx(ch, blockID)
	if bi < 0 {
		return ErrBlockNotFound
	}
	lang := cfg.Language
	original := ch.Blocks[bi].Text

	feedbackForAI := strings.TrimSpace(feedback)
	if feedbackForAI == "" {
		feedbackForAI = i18n.SystemPromptFor(lang, "segment_revision_default_feedback")
	}

	userPrompt := config.RenderPrompt(cfg.Prompts.ChapterSegmentRevision, map[string]string{
		"ChapterNum":       fmt.Sprintf("%d", ch.Num),
		"ChapterTitle":     ch.Title,
		"CorePrompt":       state.CorePrompt,
		"HistorySummary":   buildHistorySummaryForLang(state, chapterIdx, lang),
		"WritingStyle":     cfg.Story.WritingStyle,
		"WritingPOV":       cfg.Story.WritingPOV,
		"CharacterContext": buildCharacterContextForLang(settings, *ch, lang),
		"WorldviewContext": chapterWorldview(settings, *ch, lang),
		"QuotedText":       original,
		"SegmentOriginal":  original,
		"UserFeedback":     feedbackForAI,
	})
	userPrompt = appendIfMissingPlaceholder(cfg.Prompts.ChapterSegmentRevision, userPrompt, "{{.WritingPOV}}", formatWritingPOVBlock(cfg.Story.WritingPOV, lang))
	userPrompt += factProtection(state, ch.Num, lang)

	systemPrompt := state.CorePrompt
	if systemPrompt == "" {
		systemPrompt = i18n.SystemPromptFor(lang, "author_default")
	}
	systemPrompt += i18n.SystemPromptFor(lang, "chapter_revision_suffix")

	rawResp := llm.CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("block 修订 API 调用失败或被取消")
	}
	newText := strings.TrimSpace(stripChapterMetaProse(rawResp, lang))
	if newText == "" {
		return fmt.Errorf("block 修订返回空内容")
	}

	ch.Blocks[bi].Text = newText
	ch.KnowledgeTracked = true
	rebuildContentFromBlocks(ch)
	SyncChapterBlocks(ch)

	SaveChapterMarkdown(filepath.Dir(progressPath), *ch, state.Title)
	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}
	logger.SuccessKey("log.block_revised", ch.Num, blockID)
	return nil
}
