package main

// Continuation outline generation for imported / finished books.
// The v3 import pipeline itself lives in importer.go.

import (
	"context"
	"fmt"
)

func GenerateContinuationOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, settings *ProjectSettings, newChapterCount int, progressPath string, logger *LogBroadcaster) error {
	logger.StepInfo(1, 2, "正在构建已有章节上下文...")

	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN
	existingOutline := ""
	for _, ch := range state.Chapters {
		status := ""
		if ch.Status == StatusAccepted {
			status = "✅"
		}
		if en {
			existingOutline += fmt.Sprintf("Chapter %d \"%s\"%s: %s\n", ch.Num, ch.Title, status, ch.Outline)
		} else {
			existingOutline += fmt.Sprintf("第%d章《%s》%s: %s\n", ch.Num, ch.Title, status, ch.Outline)
		}
	}

	snapshot := state.StoryConfigSnapshot
	if snapshot == nil {
		snapshot = &cfg.Story
	}

	startNum := len(state.Chapters) + 1

	chapters, err := generateOutlineChaptersOnly(ctx, apiCfg, cfg, settings, cfg.Prompts.ContinuationOutlineGeneration, map[string]string{
		"Title":           state.Title,
		"StoryType":       snapshot.Type,
		"CorePrompt":      state.CorePrompt,
		"StorySynopsis":   state.StorySynopsis,
		"WritingStyle":    snapshot.WritingStyle,
		"WritingPOV":      snapshot.WritingPOV,
		"ExistingOutline": existingOutline,
		"NewChapterCount": fmt.Sprintf("%d", newChapterCount),
		"StartNum":        fmt.Sprintf("%d", startNum),
	}, logger)
	if err != nil {
		return err
	}

	logger.StepInfo(2, 2, "正在保存续写大纲...")

	for _, ch := range chapters {
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     ch.Num,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		})
	}

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	runOutlinePostProcessChecks(ctx, apiCfg, cfg, state, settings, progressPath, logger)

	logger.InfoKey("log.continuation_outline_summary", len(chapters), len(state.Chapters))
	return nil
}
