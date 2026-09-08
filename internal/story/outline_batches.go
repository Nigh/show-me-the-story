package story

import (
	"context"
	"fmt"
	"showmethestory/internal/config"
	"showmethestory/internal/i18n"
	"showmethestory/internal/sse"
	"strings"
)

// OutlineBatch binds an author's synopsis to exactly one range of chapters.
type OutlineBatch struct {
	ID                 int    `json:"id"`
	Revision           int    `json:"revision"`
	StartCh            int    `json:"start_ch"`
	EndCh              int    `json:"end_ch"`
	Synopsis           string `json:"synopsis"`
	EndingIntent       string `json:"ending_intent,omitempty"`
	EndingStyle        string `json:"ending_style,omitempty"`
	EndingRequirements string `json:"ending_requirements,omitempty"`
	PlannedFinal       bool   `json:"planned_final,omitempty"`
}

type OutlineBatchRequest struct {
	ChapterCount       int    `json:"chapter_count"`
	Synopsis           string `json:"outline_synopsis"`
	LongTermDirection  string `json:"long_term_direction"`
	Mode               string `json:"mode"`
	BatchID            int    `json:"batch_id"`
	EndingIntent       string `json:"ending_intent"`
	EndingStyle        string `json:"ending_style"`
	EndingRequirements string `json:"ending_requirements"`
	ConfirmContinue    bool   `json:"confirm_continue"`
}

// ValidateOutlineBatch never mutates the project. Call again inside the task.
func ValidateOutlineBatch(state *Progress, req OutlineBatchRequest, lang string) error {
	if err := validateEnding(state, req, lang); err != nil {
		return err
	}
	key := ""
	switch {
	case strings.TrimSpace(req.Synopsis) == "":
		key = "batch_synopsis_required"
	case req.ChapterCount < 1 || req.ChapterCount > 36:
		key = "batch_count_invalid"
	case req.Mode != "" && req.Mode != "append" && req.Mode != "replace_last":
		key = "batch_mode_invalid"
	case state.BookStatus == BookStatusCompleted:
		key = "batch_book_completed"
	}
	if key != "" {
		return fmt.Errorf("%s", i18n.T(lang, key))
	}
	for _, ch := range state.Chapters {
		if ch.Status == StatusWriting || ch.Status == StatusReview {
			return fmt.Errorf("%s", i18n.T(lang, "batch_chapter_busy"))
		}
	}
	if req.Mode == "replace_last" {
		if len(state.OutlineBatches) == 0 {
			return fmt.Errorf("%s", i18n.T(lang, "batch_replace_invalid"))
		}
		b := state.OutlineBatches[len(state.OutlineBatches)-1]
		count := 0
		for _, ch := range state.Chapters {
			if ch.Num > b.EndCh {
				return fmt.Errorf("%s", i18n.T(lang, "batch_replace_invalid"))
			}
			if ch.Num >= b.StartCh && ch.Num <= b.EndCh {
				count++
				if ch.Status != StatusPending || ch.Content != "" {
					return fmt.Errorf("%s", i18n.T(lang, "batch_replace_invalid"))
				}
			}
		}
		if b.ID != req.BatchID || count != b.EndCh-b.StartCh+1 {
			return fmt.Errorf("%s", i18n.T(lang, "batch_replace_invalid"))
		}
	}
	return nil
}

func GenerateOutlineBatch(ctx context.Context, apiCfg *config.APIConfig, cfg *config.Config, state *Progress, settings *ProjectSettings, req OutlineBatchRequest, path string, logger *sse.LogBroadcaster) error {
	if err := ValidateOutlineBatch(state, req, cfg.Language); err != nil {
		return err
	}
	next := *state
	next.Chapters = append([]ChapterState(nil), state.Chapters...)
	next.OutlineBatches = append([]OutlineBatch(nil), state.OutlineBatches...)
	id := 1
	revision := 1
	for _, b := range next.OutlineBatches {
		if b.ID >= id {
			id = b.ID + 1
		}
	}
	if req.Mode == "replace_last" {
		b := next.OutlineBatches[len(next.OutlineBatches)-1]
		id = b.ID
		revision = b.Revision + 1
		next.OutlineBatches = next.OutlineBatches[:len(next.OutlineBatches)-1]
		kept := make([]ChapterState, 0, len(next.Chapters))
		for _, ch := range next.Chapters {
			if ch.Num < b.StartCh {
				kept = append(kept, ch)
			}
		}
		next.Chapters = kept
	}
	start := 1
	var previous strings.Builder
	previous.WriteString(BatchSynopses(&next, cfg.Language))
	for _, ch := range next.Chapters {
		if ch.Num >= start {
			start = ch.Num + 1
		}
		previous.WriteString(formatChapterLine(ch.Num, ch.Title, ch.Outline, cfg.Language))
		if ch.Summary != "" {
			previous.WriteString(ch.Summary + "\n")
		}
	}
	synopsis := strings.TrimSpace(req.Synopsis)
	template := cfg.Prompts.ContinuationOutlineGeneration
	// Always add the explicit scope, including for previously saved custom templates.
	template += batchScopeTemplate(cfg.Language)
	template += endingPrompt(OutlineBatch{EndCh: start + req.ChapterCount - 1, EndingIntent: req.EndingIntent, EndingStyle: req.EndingStyle, EndingRequirements: req.EndingRequirements, PlannedFinal: req.EndingIntent == "final" || req.EndingIntent == "sequel"}, 0, cfg.Language)
	data := map[string]string{
		"Title": preferUserValue(cfg.Story.Title, state.Title), "StoryType": cfg.Story.Type,
		"CorePrompt": state.CorePrompt, "StorySynopsis": synopsis, "OutlineSynopsis": synopsis,
		"WritingStyle": cfg.Story.WritingStyle, "WritingPOV": cfg.Story.WritingPOV,
		"ExistingOutline": previous.String(), "NewChapterCount": fmt.Sprint(req.ChapterCount),
		"StartNum": fmt.Sprint(start), "EndNum": fmt.Sprint(start + req.ChapterCount - 1),
		"UserRequirements": synopsis, "LongTermDirection": strings.TrimSpace(req.LongTermDirection),
	}
	var chapters []OutlineChapter
	for attempt := 0; attempt < 3; attempt++ {
		var err error
		chapters, err = generateOutlineChaptersOnly(ctx, apiCfg, cfg, settings, template, data, logger)
		if err != nil {
			return err
		}
		valid := len(chapters) == req.ChapterCount
		for i, ch := range chapters {
			if ch.Num != start+i {
				valid = false
			}
		}
		if valid {
			break
		}
		if attempt == 2 {
			return fmt.Errorf("%s", i18n.T(cfg.Language, "batch_response_invalid"))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, ch := range chapters {
		next.Chapters = append(next.Chapters, chapterStateFromOutline(ch, StatusPending))
	}
	for i := range next.OutlineBatches {
		next.OutlineBatches[i].PlannedFinal = false
	}
	next.OutlineBatches = append(next.OutlineBatches, OutlineBatch{ID: id, Revision: revision, StartCh: start, EndCh: start + req.ChapterCount - 1, Synopsis: synopsis, EndingIntent: req.EndingIntent, EndingStyle: req.EndingStyle, EndingRequirements: strings.TrimSpace(req.EndingRequirements), PlannedFinal: req.EndingIntent == "final" || req.EndingIntent == "sequel"})
	next.Title = preferUserValue(cfg.Story.Title, state.Title)
	next.LongTermDirection = strings.TrimSpace(req.LongTermDirection)
	next.Phase = "writing"
	next.BookStatus = BookStatusActive
	if err := SaveProgress(path, &next); err != nil {
		return err
	}
	*state = next
	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, path, logger)
	return nil
}
