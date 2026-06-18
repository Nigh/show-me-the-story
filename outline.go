package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type OutlineResponse struct {
	Title        string           `json:"title"`
	CorePrompt   string           `json:"core_prompt"`
	StorySynopsis string          `json:"story_synopsis"`
	Chapters     []OutlineChapter `json:"chapters"`
}

type OutlineChapter struct {
	Num     int    `json:"num"`
	Title   string `json:"title"`
	Outline string `json:"outline"`
}

func generateOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config) (*OutlineResponse, error) {
	chapterCountStr := fmt.Sprintf("%d", cfg.Story.ChapterCount)
	targetWordsStr := fmt.Sprintf("%d", cfg.Story.TargetWordsPerChapter)

	userPrompt := RenderPrompt(cfg.Prompts.OutlineGeneration, map[string]string{
		"StoryType":        cfg.Story.Type,
		"ChapterCount":     chapterCountStr,
		"TargetWords":      targetWordsStr,
		"WritingStyle":     cfg.Story.WritingStyle,
		"WritingPOV":       cfg.Story.WritingPOV,
		"StorySynopsis":    cfg.Story.StorySynopsis,
	})

	systemPrompt := SystemPromptFor(cfg.Language, "outline_editor_json")

	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return nil, fmt.Errorf("API 调用失败或被取消")
	}

	rawResp = cleanJSONResponse(rawResp)

	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return nil, fmt.Errorf("解析大纲JSON失败: %w\n原始响应: %s", err, rawResp)
	}

	return &resp, nil
}

func reviseOutline(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, userFeedback string) error {
	lang := cfg.Language
	en := NormalizeLanguage(lang) == LangEN

	lockedChapters := ""
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedChapters += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
		}
	}
	if lockedChapters == "" {
		if en {
			lockedChapters = "(no locked chapters)"
		} else {
			lockedChapters = "无已锁定章节。"
		}
	}

	currentOutline := ""
	for _, ch := range state.Chapters {
		currentOutline += formatChapterLine(ch.Num, ch.Title, ch.Outline, lang)
	}

	userPrompt := RenderPrompt(cfg.Prompts.OutlineRevision, map[string]string{
		"CurrentOutline": currentOutline,
		"UserFeedback":   userFeedback,
		"LockedChapters": lockedChapters,
	})

	systemPrompt := SystemPromptFor(lang, "outline_editor_locked_json")

	rawResp := CallAPIWithRetry(ctx, apiCfg, systemPrompt, userPrompt)
	if rawResp == "" {
		return fmt.Errorf("API 调用失败或被取消")
	}
	rawResp = cleanJSONResponse(rawResp)

	var resp OutlineResponse
	if err := json.Unmarshal([]byte(rawResp), &resp); err != nil {
		return fmt.Errorf("解析修订大纲JSON失败: %w\n原始响应: %s", err, rawResp)
	}

	applyOutlineRevision(resp, state)

	return nil
}

func applyOutlineRevision(resp OutlineResponse, state *Progress) {
	lockedMap := make(map[int]bool)
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			lockedMap[ch.Num] = true
		}
	}

	for _, newCh := range resp.Chapters {
		for i, existingCh := range state.Chapters {
			if existingCh.Num == newCh.Num && !lockedMap[newCh.Num] {
				state.Chapters[i].Title = newCh.Title
				state.Chapters[i].Outline = newCh.Outline
			}
		}
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
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func GenerateOutlineAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath string, logger *LogBroadcaster) error {
	if err := validateAPIConfig(apiCfg); err != nil {
		return err
	}
	// 防护：整体生成大纲会覆盖全部章节，存在已确认章节时绝不允许，
	// 否则会静默抹掉已写完的内容。续写场景请使用 GenerateContinuationOutline。
	for _, ch := range state.Chapters {
		if ch.Status == StatusAccepted {
			return fmt.Errorf("存在已确认章节，无法整体重新生成大纲（会覆盖已完成内容）。如需追加章节请使用「生成后续大纲」")
		}
	}

	logger.StepInfo(1, 2, "正在调用 AI 生成大纲...")

	outlineResp, err := generateOutline(ctx, apiCfg, cfg)
	if err != nil {
		return fmt.Errorf("生成大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存大纲...")

	state.Title = outlineResp.Title
	state.CorePrompt = outlineResp.CorePrompt
	state.StorySynopsis = outlineResp.StorySynopsis
	state.Chapters = make([]ChapterState, len(outlineResp.Chapters))
	for i, ch := range outlineResp.Chapters {
		state.Chapters[i] = ChapterState{
			Num:     ch.Num,
			Title:   ch.Title,
			Outline: ch.Outline,
			Status:  StatusPending,
		}
	}
	snapshot := cfg.Story
	state.StoryConfigSnapshot = &snapshot

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	logger.SuccessKey("log.outline_generate_summary", len(state.Chapters), state.Title)
	return nil
}

func ReviseOutlineAction(ctx context.Context, apiCfg *APIConfig, cfg *Config, state *Progress, progressPath, feedback string, logger *LogBroadcaster) error {
	logger.StepInfo(1, 2, "正在根据意见修订大纲...")

	if err := reviseOutline(ctx, apiCfg, cfg, state, feedback); err != nil {
		return fmt.Errorf("修订大纲失败: %w", err)
	}

	logger.StepInfo(2, 2, "正在保存修订后的大纲...")

	if err := SaveProgress(progressPath, state); err != nil {
		return fmt.Errorf("保存进度失败: %w", err)
	}

	RunForeshadowOutlineCheckAndSave(ctx, apiCfg, cfg, state, progressPath, logger)

	logger.SuccessKey("log.outline_revise_summary", len(state.Chapters))
	return nil
}

func ConfirmOutlineAction(state *Progress, progressPath string) error {
	if len(state.Chapters) == 0 {
		return fmt.Errorf("大纲为空")
	}

	state.Phase = "writing"
	return SaveProgress(progressPath, state)
}

func EditChapterOutline(state *Progress, chapterNum int, title, outline string) error {
	idx := -1
	for i, ch := range state.Chapters {
		if ch.Num == chapterNum {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("章节 %d 不存在", chapterNum)
	}
	if state.Chapters[idx].Status != StatusPending {
		return fmt.Errorf("只能编辑待定（pending）状态的章节大纲")
	}
	state.Chapters[idx].Title = title
	state.Chapters[idx].Outline = outline
	return nil
}
