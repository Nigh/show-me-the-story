package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ContinueAnalysis struct {
	Title         string            `json:"title"`
	StoryType     string            `json:"story_type"`
	CorePrompt    string            `json:"core_prompt"`
	StorySynopsis string            `json:"story_synopsis"`
	WritingStyle  string            `json:"writing_style"`
	WritingPOV    string            `json:"writing_pov"`
	Chapters      []ContinueChapter `json:"chapters"`
}

type ContinueChapter struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline,omitempty"`
	Summary string `json:"summary,omitempty"`
	Content string `json:"content,omitempty"`
}

func AnalyzeExistingContent(ctx context.Context, apiCfg *APIConfig, cfg *Config, content string) (*ContinueAnalysis, error) {
	userPrompt := RenderPrompt(cfg.Prompts.ContentAnalysis, map[string]string{
		"ExistingContent": content,
	})

	systemPrompt := SystemPromptFor(cfg.Language, "content_analyst_json")

	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp ContinueAnalysis
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析分析结果JSON失败: %w", err)
	}

	return &resp, nil
}

func splitContentByChapters(content string, chapters []ContinueChapter) []string {
	if len(chapters) == 0 {
		return nil
	}

	re := regexp.MustCompile(`(?m)^[\s]*(第[一二三四五六七八九十百千\d]+章|Chapter\s+\d+|#\s+Chapter\s+\d+|第\d+章)`)
	matches := re.FindAllStringIndex(content, -1)

	if len(matches) == 0 {
		return []string{content}
	}

	segments := make([]string, 0, len(matches))
	for i, match := range matches {
		start := match[0]
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		seg := strings.TrimSpace(content[start:end])
		if seg != "" {
			segments = append(segments, seg)
		}
	}

	if len(segments) == 0 {
		return []string{content}
	}

	return segments
}

func ImportContinueAction(cfg *Config, state *Progress, analysis *ContinueAnalysis, content string, progressPath string, cfgPath string) error {
	state.Title = analysis.Title
	state.CorePrompt = analysis.CorePrompt
	state.StorySynopsis = analysis.StorySynopsis

	segments := splitContentByChapters(content, analysis.Chapters)

	state.Chapters = make([]ChapterState, 0, len(analysis.Chapters))
	for i, ch := range analysis.Chapters {
		chapterContent := ""
		if i < len(segments) {
			chapterContent = segments[i]
		}
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     i + 1,
			Title:   ch.Title,
			Outline: ch.Outline,
			Content: chapterContent,
			Summary: ch.Summary,
			Status:  StatusAccepted,
		})
	}

	state.CurrentChapterIndex = len(analysis.Chapters)
	state.Phase = "outline"

	snapshot := StoryConfig{
		Type:                  analysis.StoryType,
		Title:                 analysis.Title,
		ChapterCount:          len(state.Chapters),
		TargetWordsPerChapter: cfg.Story.TargetWordsPerChapter,
		WritingStyle:          analysis.WritingStyle,
		WritingPOV:            analysis.WritingPOV,
		StorySynopsis:         analysis.StorySynopsis,
	}
	state.StoryConfigSnapshot = &snapshot

	oldStory := cfg.Story

	cfg.Story.Type = analysis.StoryType
	cfg.Story.Title = analysis.Title
	cfg.Story.WritingStyle = analysis.WritingStyle
	cfg.Story.WritingPOV = analysis.WritingPOV
	cfg.Story.StorySynopsis = analysis.StorySynopsis

	if err := SaveProgress(progressPath, state); err != nil {
		cfg.Story = oldStory
		return fmt.Errorf("保存进度失败: %w", err)
	}

	if err := saveConfig(cfgPath, cfg); err != nil {
		cfg.Story = oldStory
		return fmt.Errorf("保存配置失败: %w", err)
	}

	return nil
}

func GenerateContinuationOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, newChapterCount int, progressPath string, logger *LogBroadcaster) error {
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

	userPrompt := RenderPrompt(cfg.Prompts.ContinuationOutlineGeneration, map[string]string{
		"Title":            state.Title,
		"StoryType":        snapshot.Type,
		"CorePrompt":       state.CorePrompt,
		"StorySynopsis":    state.StorySynopsis,
		"WritingStyle":     snapshot.WritingStyle,
		"WritingPOV":       snapshot.WritingPOV,
		"ExistingOutline":  existingOutline,
		"NewChapterCount":  fmt.Sprintf("%d", newChapterCount),
		"StartNum":         fmt.Sprintf("%d", startNum),
	})

	systemPrompt := SystemPromptFor(lang, "outline_editor_json")

	rawResp := CallAPIWithRetryLog(ctx, apiCfg, systemPrompt, userPrompt, logger)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析续写大纲JSON失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存续写大纲...")

	for _, ch := range resp.Chapters {
		state.Chapters = append(state.Chapters, ChapterState{
			Num:     ch.Num,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		})
	}

	if resp.Title != "" {
		state.Title = resp.Title
	}
	if resp.CorePrompt != "" {
		state.CorePrompt = resp.CorePrompt
	}
	if resp.StorySynopsis != "" {
		state.StorySynopsis = resp.StorySynopsis
	}

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	RunForeshadowOutlineCheckAndSave(ctx, apiCfg, cfg, state, progressPath, logger)

	logger.InfoKey("log.continuation_outline_summary", len(resp.Chapters), len(state.Chapters))
	return nil
}
